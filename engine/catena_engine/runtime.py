"""Role turns on the owner's LLM through the OpenAI Agents SDK."""

from __future__ import annotations

import asyncio
import time
from collections.abc import Callable
from typing import Any

from agents import Agent, Model, ModelSettings, OpenAIChatCompletionsModel, RunConfig, Runner, set_tracing_disabled
from openai import AsyncOpenAI

from .protocol import ModelConfig, TurnRequest
from .roles import ROLES

# The SDK exports traces to OpenAI by default. Owner evidence and keys must
# never leave through that path, so tracing stays disabled process-wide.
set_tracing_disabled(True)

ModelFactory = Callable[[ModelConfig], tuple[Model, Callable[[], Any] | None]]


def openai_compatible_model(config: ModelConfig) -> tuple[Model, Callable[[], Any]]:
    """Chat Completions is the lowest common denominator across BYOK providers."""
    client = AsyncOpenAI(api_key=config.api_key, base_url=config.base_url, max_retries=1)
    return OpenAIChatCompletionsModel(model=config.model, openai_client=client), client.close


async def run_turn(request: TurnRequest, model_factory: ModelFactory = openai_compatible_model) -> dict[str, Any]:
    role = ROLES[request.role]
    model, close = model_factory(request.model)
    agent = Agent(
        name=role.display_name,
        instructions=role.instructions,
        model=model,
        model_settings=ModelSettings(temperature=0.2),
    )
    started = time.monotonic()
    try:
        result = await asyncio.wait_for(
            Runner.run(agent, request.prompt, max_turns=1, run_config=RunConfig(tracing_disabled=True)),
            timeout=request.timeout_ms / 1000,
        )
    except TimeoutError:
        return _failed("timeout", "the model did not answer before the turn timeout", started)
    except Exception as error:  # provider and SDK errors are reported, never raised to the caller
        return _failed("model_error", _safe_detail(error, request.model.api_key), started)
    finally:
        if close is not None:
            await close()

    content = result.final_output if isinstance(result.final_output, str) else ""
    if not content.strip():
        return _failed("empty_output", "the model returned no text", started)
    usage = result.context_wrapper.usage
    return {
        "status": "completed",
        "assistant": {"role": "assistant", "content": content},
        "usage": {
            "requests": usage.requests,
            "input_tokens": usage.input_tokens,
            "output_tokens": usage.output_tokens,
        },
        "duration_ms": _elapsed_ms(started),
    }


def _failed(reason: str, detail: str, started: float) -> dict[str, Any]:
    return {"status": "failed", "reason_code": reason, "detail": detail, "duration_ms": _elapsed_ms(started)}


def _elapsed_ms(started: float) -> int:
    return int((time.monotonic() - started) * 1000)


def _safe_detail(error: Exception, api_key: str) -> str:
    detail = f"{type(error).__name__}: {error}"
    if api_key:
        detail = detail.replace(api_key, "[redacted]")
    return detail[:500]
