"""Cancel host operations, awaiting their own process/environment cleanup."""

import asyncio

from .contracts import OptimizationError


async def cancellable(operation, cancelled):
    task = asyncio.create_task(operation)
    try:
        while not task.done():
            if cancelled is not None and cancelled.is_set():
                raise OptimizationError("RSI cancelled")
            await asyncio.wait((task,), timeout=0.05)
        return await task
    finally:
        if not task.done():
            task.cancel()
        await asyncio.gather(task, return_exceptions=True)
