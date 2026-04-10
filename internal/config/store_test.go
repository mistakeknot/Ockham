package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mistakeknot/Ockham/internal/config"
)

func TestStore_LoadMissing_ReturnsDefault(t *testing.T) {
	s := config.NewStore(filepath.Join(t.TempDir(), "ockham.yaml"))
	f, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if f.Version != 1 {
		t.Errorf("expected version 1, got %d", f.Version)
	}
	if len(f.Orgs) == 0 {
		t.Error("expected orgs in default config")
	}
}

func TestStore_SaveAndLoad_Roundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ockham.yaml")
	s := config.NewStore(path)
	orig := config.DefaultConfig()
	orig.Orgs = []config.Org{{Name: "test-org"}}

	if err := s.Save(orig); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Orgs) != 1 || loaded.Orgs[0].Name != "test-org" {
		t.Errorf("expected single org 'test-org', got %v", loaded.Orgs)
	}
}

func TestStore_Save_AtomicReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ockham.yaml")
	s := config.NewStore(path)

	f := config.DefaultConfig()
	if err := s.Save(f); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file at %s after atomic save", path)
	}
}

func TestStore_LoadCorrupt_ReturnsDefaultWithError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ockham.yaml")
	if err := os.WriteFile(path, []byte("{{{{not yaml"), 0644); err != nil {
		t.Fatal(err)
	}
	s := config.NewStore(path)
	f, err := s.Load()
	if err == nil {
		t.Error("expected error for corrupt YAML")
	}
	if f.Version != 1 {
		t.Error("corrupt file should still return default as fallback")
	}
}

func TestValidate_EmptyOrgs(t *testing.T) {
	f := config.DefaultConfig()
	f.Orgs = nil
	err := config.Validate(f)
	if err == nil {
		t.Error("expected validation error for empty orgs")
	}
	if !strings.Contains(err.Error(), "orgs must not be empty") {
		t.Errorf("expected 'orgs must not be empty' in error, got: %s", err)
	}
}

func TestValidate_EmptyOrgName(t *testing.T) {
	f := config.DefaultConfig()
	f.Orgs = []config.Org{{Name: ""}}
	err := config.Validate(f)
	if err == nil {
		t.Error("expected validation error for empty org name")
	}
	if !strings.Contains(err.Error(), "name must not be empty") {
		t.Errorf("expected 'name must not be empty' in error, got: %s", err)
	}
}

func TestValidate_BadVersion(t *testing.T) {
	f := config.DefaultConfig()
	f.Version = 99
	err := config.Validate(f)
	if err == nil {
		t.Error("expected validation error for bad version")
	}
	if !strings.Contains(err.Error(), "unsupported config version") {
		t.Errorf("expected 'unsupported config version' in error, got: %s", err)
	}
}

func TestValidate_MissingTier(t *testing.T) {
	f := config.DefaultConfig()
	delete(f.Cost.Tiers, 2)
	err := config.Validate(f)
	if err == nil {
		t.Error("expected validation error for missing tier")
	}
	if !strings.Contains(err.Error(), "cost tier 2 not defined") {
		t.Errorf("expected 'cost tier 2 not defined' in error, got: %s", err)
	}
}

func TestValidate_AgentBadTier(t *testing.T) {
	f := config.DefaultConfig()
	f.Agents["bad"] = config.AgentEntry{
		Model:    "test",
		CostTier: 5,
		Runtime:  "test",
	}
	err := config.Validate(f)
	if err == nil {
		t.Error("expected validation error for agent with bad cost tier")
	}
	if !strings.Contains(err.Error(), "out of range") {
		t.Errorf("expected 'out of range' in error, got: %s", err)
	}
}

func TestValidate_AgentEmptyRuntime(t *testing.T) {
	f := config.DefaultConfig()
	f.Agents["bad"] = config.AgentEntry{
		Model:    "test",
		CostTier: 0,
		Runtime:  "",
	}
	err := config.Validate(f)
	if err == nil {
		t.Error("expected validation error for agent with empty runtime")
	}
	if !strings.Contains(err.Error(), "runtime must not be empty") {
		t.Errorf("expected 'runtime must not be empty' in error, got: %s", err)
	}
}

func TestValidate_ValidDefault(t *testing.T) {
	f := config.DefaultConfig()
	if err := config.Validate(f); err != nil {
		t.Errorf("expected no errors, got %v", err)
	}
}

func TestStore_Path(t *testing.T) {
	s := config.NewStore("/tmp/test.yaml")
	if s.Path() != "/tmp/test.yaml" {
		t.Errorf("expected /tmp/test.yaml, got %s", s.Path())
	}
}
