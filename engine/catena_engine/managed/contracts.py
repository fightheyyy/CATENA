"""Private Go-to-OpenSandbox request contracts."""

from dataclasses import dataclass


@dataclass(frozen=True)
class SandboxBinding:
    run_id: str
    sandbox_id: str
    provider_id: str
    session_id: str
    tenant_id: str
    generation: int
    fencing_token: int
    token: str
    namespace: str

    def __post_init__(self):
        if (
            not all(isinstance(value, str) and value for value in (
                self.run_id, self.sandbox_id, self.session_id, self.tenant_id, self.namespace
            ))
            or len(self.token) != 43
            or type(self.generation) is not int
            or type(self.fencing_token) is not int
            or min(self.generation, self.fencing_token) < 1
        ):
            raise ValueError("invalid Sandbox binding")


@dataclass(frozen=True)
class Create(SandboxBinding):
    cpu: str
    memory: str


@dataclass(frozen=True)
class Turn(SandboxBinding):
    input: str
    profile: str
    profile_version: int
    instructions: str
    tool_policy: str
    skills: list
    knowledge_refs: list
    model: str
    timeout: int
    gateway: str
    tool_gateway: str
    checkpoint: str

    def __post_init__(self):
        super().__post_init__()
        if (
            not isinstance(self.provider_id, str)
            or not self.provider_id
            or self.profile not in ("managed", "analyze", "improve")
            or type(self.profile_version) is not int
            or self.profile_version < 1
            or type(self.timeout) is not int
            or not 1 <= self.timeout <= 1800
            or not isinstance(self.input, str)
            or len(self.input.encode()) > 1_000_000
            or not isinstance(self.skills, list)
            or not isinstance(self.knowledge_refs, list)
            or self.tool_policy not in ("sandbox-write", "read-only")
        ):
            raise ValueError("invalid Sandbox turn")


@dataclass(frozen=True)
class Namespace:
    namespace: str


@dataclass(frozen=True)
class SandboxRef:
    provider_id: str
