"""Explicit registration, with no import-by-name or executable plugin discovery."""

from .contracts import OptimizationError, RSIAdapter


class Registry:
    def __init__(self):
        self._adapters: dict[str, RSIAdapter] = {}

    def register(self, adapter: RSIAdapter) -> None:
        if adapter.name in self._adapters:
            raise OptimizationError("duplicate RSI adapter")
        self._adapters[adapter.name] = adapter

    def get(self, name: str) -> RSIAdapter:
        if name not in self._adapters:
            raise OptimizationError("unknown RSI adapter")
        return self._adapters[name]

    def describe(self) -> list[dict]:
        return [{"name": a.name, "version": a.version} for a in self._adapters.values()]


def default_registry() -> Registry:
    from .harness2 import Harness2Adapter

    registry = Registry()
    registry.register(Harness2Adapter())
    return registry
