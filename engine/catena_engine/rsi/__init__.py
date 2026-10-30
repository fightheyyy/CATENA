"""Host-owned optimization contracts; algorithm dependencies are loaded lazily."""

from .contracts import Algorithm, Budget, Candidate, Context, OptimizationResult, RSIAdapter
from .registry import Registry

__all__ = ["Algorithm", "Budget", "Candidate", "Context", "OptimizationResult", "RSIAdapter", "Registry"]
