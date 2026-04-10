package config_test

import (
	"testing"

	"github.com/mistakeknot/Ockham/internal/config"
)

func TestDefaultConfig_Valid(t *testing.T) {
	cfg := config.DefaultConfig()
	if err := config.Validate(cfg); err != nil {
		t.Errorf("DefaultConfig should be valid, got: %v", err)
	}
}

func TestDefaultConfig_HasOrgs(t *testing.T) {
	cfg := config.DefaultConfig()
	if len(cfg.Orgs) == 0 {
		t.Error("expected at least one org in default config")
	}
}

func TestDefaultConfig_HasAgents(t *testing.T) {
	cfg := config.DefaultConfig()
	if len(cfg.Agents) == 0 {
		t.Error("expected at least one agent in default config")
	}
	for name, agent := range cfg.Agents {
		if agent.Runtime == "" {
			t.Errorf("agent %q: runtime should not be empty", name)
		}
	}
}

func TestDefaultConfig_HasAllTiers(t *testing.T) {
	cfg := config.DefaultConfig()
	for tier := 0; tier <= 3; tier++ {
		if _, ok := cfg.Cost.Tiers[tier]; !ok {
			t.Errorf("expected cost tier %d in default config", tier)
		}
	}
}

func TestDefaultConfig_Version(t *testing.T) {
	cfg := config.DefaultConfig()
	if cfg.Version != 1 {
		t.Errorf("expected version 1, got %d", cfg.Version)
	}
}
