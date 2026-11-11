package control

type sandboxAction string

const (
	sandboxCreate    sandboxAction = "create"
	sandboxExecute   sandboxAction = "execute"
	sandboxList      sandboxAction = "list"
	sandboxPause     sandboxAction = "pause"
	sandboxResume    sandboxAction = "resume"
	sandboxTerminate sandboxAction = "terminate"
)

type sandboxBinding struct {
	RunID      string `json:"run_id"`
	SandboxID  string `json:"sandbox_id"`
	ProviderID string `json:"provider_id"`
	SessionID  string `json:"session_id"`
	TenantID   string `json:"tenant_id"`
	Generation int64  `json:"generation"`
	Fence      int64  `json:"fencing_token"`
	Token      string `json:"token"`
	Namespace  string `json:"namespace"`
}

type sandboxCreateRequest struct {
	sandboxBinding
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}

type sandboxRef struct {
	ProviderID string `json:"provider_id"`
}

type sandboxNamespace struct {
	Namespace string `json:"namespace"`
}

type sandboxTurn struct {
	sandboxBinding
	Input          string         `json:"input"`
	Profile        string         `json:"profile"`
	ProfileVersion int            `json:"profile_version"`
	Instructions   string         `json:"instructions"`
	ToolPolicy     string         `json:"tool_policy"`
	Skills         []harnessSkill `json:"skills"`
	Knowledge      []string       `json:"knowledge_refs"`
	Model          string         `json:"model"`
	Timeout        int            `json:"timeout"`
	Gateway        string         `json:"gateway"`
	ToolGateway    string         `json:"tool_gateway"`
	Checkpoint     string         `json:"checkpoint"`
}

func (p *managedPlatform) binding(l *managedLease) sandboxBinding {
	return sandboxBinding{
		RunID: l.Run.ID, SandboxID: l.Box.ID, ProviderID: l.Box.Provider, SessionID: l.Run.Session,
		TenantID: l.Run.Tenant, Generation: l.Box.Generation, Fence: l.Run.Fence, Token: l.Token,
		Namespace: p.config.Namespace,
	}
}
