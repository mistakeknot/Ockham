package config

// ConfigFile is the on-disk YAML representation of ockham.yaml.
type ConfigFile struct {
	Version int          `yaml:"version"`
	Orgs    []Org        `yaml:"orgs"`
	Agents  Agents       `yaml:"agents"`
	Cost    CostConfig   `yaml:"cost"`
	Notify  NotifyConfig `yaml:"notify"`
}

// Org represents a GitHub organization to discover work from.
type Org struct {
	Name   string `yaml:"name"`
	Prefix string `yaml:"prefix,omitempty"`
	SAML   bool   `yaml:"saml,omitempty"`
}

// AgentEntry describes a dispatchable agent.
type AgentEntry struct {
	Model        string   `yaml:"model"`
	CostTier     int      `yaml:"cost_tier"`
	Capabilities []string `yaml:"capabilities"`
	Runtime      string   `yaml:"runtime"`
}

// Agents maps agent name to configuration.
type Agents map[string]AgentEntry

// CostTierDef defines a cost tier.
type CostTierDef struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	AutoApprove bool   `yaml:"auto_approve"`
	Requires    string `yaml:"requires,omitempty"`
}

// CostConfig holds tier definitions.
type CostConfig struct {
	Tiers map[int]CostTierDef `yaml:"tiers"`
}

// TelegramConfig for notifications.
type TelegramConfig struct {
	Enabled bool   `yaml:"enabled"`
	ChatID  string `yaml:"chat_id,omitempty"`
}

// QuietHours defines when not to notify.
type QuietHours struct {
	Start    string `yaml:"start"`
	End      string `yaml:"end"`
	Timezone string `yaml:"timezone"`
}

// DigestConfig for daily summaries.
type DigestConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Schedule string `yaml:"schedule"`
}

// NotifyConfig holds notification settings.
type NotifyConfig struct {
	Telegram   TelegramConfig `yaml:"telegram"`
	QuietHours QuietHours     `yaml:"quiet_hours"`
	Digest     DigestConfig   `yaml:"digest"`
}

// DefaultConfig returns the hardcoded fallback configuration.
func DefaultConfig() ConfigFile {
	return ConfigFile{
		Version: 1,
		Orgs: []Org{
			{Name: "mistakeknot"},
			{Name: "gensysven"},
			{Name: "vibeguider"},
			{Name: "sojrn"},
			{Name: "yes-rly"},
			{Name: "wmpcx", SAML: true},
			{Name: "WM-MergersAcquisitions", SAML: true},
		},
		Agents: Agents{
			"hermes": {
				Model:        "glm-5",
				CostTier:     0,
				Capabilities: []string{"research", "planning", "tool-calls", "beads"},
				Runtime:      "hermes",
			},
			"claude-code": {
				Model:        "claude",
				CostTier:     2,
				Capabilities: []string{"coding", "refactoring", "debugging", "review"},
				Runtime:      "claude-code",
			},
			"codex": {
				Model:        "gpt-5.4",
				CostTier:     3,
				Capabilities: []string{"coding", "prototyping", "web-search"},
				Runtime:      "codex",
			},
			"skaffen": {
				Model:        "local",
				CostTier:     0,
				Capabilities: []string{"execution", "oodarc"},
				Runtime:      "skaffen",
			},
		},
		Cost: CostConfig{
			Tiers: map[int]CostTierDef{
				0: {Name: "free", Description: "GLM-5 tool calls, local MLX, git ops, file reads", AutoApprove: true},
				1: {Name: "cheap", Description: "Embedding calls, small model summaries", AutoApprove: true},
				2: {Name: "medium", Description: "Claude Code interactive, per-session approval", AutoApprove: false, Requires: "session_approval"},
				3: {Name: "expensive", Description: "Claude -p autonomous, Codex exec", AutoApprove: false, Requires: "explicit_approval"},
			},
		},
		Notify: NotifyConfig{
			Telegram: TelegramConfig{Enabled: true},
			QuietHours: QuietHours{
				Start:    "23:00",
				End:      "07:00",
				Timezone: "America/New_York",
			},
			Digest: DigestConfig{
				Enabled:  true,
				Schedule: "09:00",
			},
		},
	}
}
