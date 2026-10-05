from __future__ import annotations

import asyncio
from collections.abc import AsyncIterator
from typing import Any

import pytest
from agents import Model, ModelResponse, Usage
from openai.types.responses import ResponseOutputMessage, ResponseOutputText

from catena_engine.protocol import REQUEST_SCHEMA


class FakeModel(Model):
    """Answers with a fixed text, raises, or stalls, and records what it was asked."""

    def __init__(self, text: str = '{"ok":true}', error: Exception | None = None, delay: float = 0) -> None:
        self.text = text
        self.error = error
        self.delay = delay
        self.calls: list[dict[str, Any]] = []

    async def get_response(
        self, system_instructions, input, model_settings, tools, output_schema, handoffs, tracing, **_
    ):
        self.calls.append({"instructions": system_instructions, "input": input, "tools": tools})
        if self.delay:
            await asyncio.sleep(self.delay)
        if self.error:
            raise self.error
        message = ResponseOutputMessage(
            id="msg_1",
            type="message",
            role="assistant",
            status="completed",
            content=[ResponseOutputText(type="output_text", text=self.text, annotations=[])],
        )
        return ModelResponse(
            output=[message],
            usage=Usage(requests=1, input_tokens=11, output_tokens=7, total_tokens=18),
            response_id=None,
        )

    def stream_response(self, *args, **kwargs) -> AsyncIterator[Any]:
        raise NotImplementedError


def factory_for(model: FakeModel):
    closed: list[bool] = []

    async def close() -> None:
        closed.append(True)

    def factory(_config):
        return model, close

    factory.closed = closed
    return factory


@pytest.fixture
def turn_payload() -> dict[str, Any]:
    return {
        "schema": REQUEST_SCHEMA,
        "request_id": "inspector-1",
        "run_id": "job-1",
        "role": "inspector",
        "prompt": "Analyze the evidence.",
        "timeout_ms": 5000,
        "model": {
            "provider": "deepseek",
            "base_url": "https://api.example.test/v1",
            "model": "example-model",
            "api_key": "sk-test-secret-value",
        },
    }
