package intent_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mistakeknot/Ockham/internal/intent"
)

func TestStore_LoadMissing_ReturnsDefault(t *testing.T) {
	s := intent.NewStore(filepath.Join(t.TempDir(), "intent.yaml"))
	f, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Themes) != 1 {
		t.Errorf("expected 1 default theme, got %d", len(f.Themes))
	}
	if _, ok := f.Themes["open"]; !ok {
		t.Error("expected 'open' theme in default")
	}
}

func TestStore_SaveAndLoad_Roundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "intent.yaml")
	s := intent.NewStore(path)
	orig := intent.IntentFile{
		Version: 1,
		Themes: map[string]intent.ThemeBudget{
			"auth": {Budget: 0.6, Priority: intent.PriorityHigh},
			"open": {Budget: 0.4, Priority: intent.PriorityNormal},
		},
	}
	if err := s.Save(orig); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Themes["auth"].Budget != 0.6 {
		t.Errorf("expected auth budget 0.6, got %f", loaded.Themes["auth"].Budget)
	}
}

func TestStore_Save_AtomicReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "intent.yaml")
	s := intent.NewStore(path)

	f := intent.DefaultFile()
	if err := s.Save(f); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file at %s after atomic save", path)
	}
}

func TestValidate_BudgetsSumToOne(t *testing.T) {
	f := intent.IntentFile{
		Version: 1,
		Themes: map[string]intent.ThemeBudget{
			"a": {Budget: 0.5, Priority: intent.PriorityNormal},
			"b": {Budget: 0.3, Priority: intent.PriorityNormal},
		},
	}
	if err := intent.Validate(f); err == nil {
		t.Error("expected validation error for budgets summing to 0.8")
	}
}

func TestValidate_NegativeBudget(t *testing.T) {
	f := intent.IntentFile{
		Version: 1,
		Themes: map[string]intent.ThemeBudget{
			"a": {Budget: -0.1, Priority: intent.PriorityNormal},
			"b": {Budget: 1.1, Priority: intent.PriorityNormal},
		},
	}
	err := intent.Validate(f)
	if err == nil {
		t.Error("expected validation error for negative budget")
	}
	if !strings.Contains(err.Error(), "out of range") {
		t.Errorf("expected 'out of range' in error, got: %s", err)
	}
}

func TestValidate_FreezeUnknownTheme(t *testing.T) {
	f := intent.IntentFile{
		Version: 1,
		Themes: map[string]intent.ThemeBudget{
			"auth": {Budget: 1.0, Priority: intent.PriorityNormal},
		},
		Constraints: intent.Constraints{
			Freeze: []string{"typo-theme"},
		},
	}
	if err := intent.Validate(f); err == nil {
		t.Error("expected validation error for unknown freeze theme")
	}
}

func TestValidate_InvalidPriority(t *testing.T) {
	f := intent.IntentFile{
		Version: 1,
		Themes: map[string]intent.ThemeBudget{
			"auth": {Budget: 1.0, Priority: "urgent"},
		},
	}
	err := intent.Validate(f)
	if err == nil {
		t.Error("expected validation error for unrecognized priority")
	}
	if !strings.Contains(err.Error(), "unknown priority") {
		t.Errorf("expected 'unknown priority' in error, got: %s", err)
	}
}

func TestValidate_ValidFile(t *testing.T) {
	f := intent.IntentFile{
		Version: 1,
		Themes: map[string]intent.ThemeBudget{
			"auth": {Budget: 0.4, Priority: intent.PriorityHigh},
			"perf": {Budget: 0.3, Priority: intent.PriorityNormal},
			"open": {Budget: 0.3, Priority: intent.PriorityNormal},
		},
		Constraints: intent.Constraints{
			Freeze: []string{"auth"},
		},
	}
	if err := intent.Validate(f); err != nil {
		t.Errorf("expected no errors, got %v", err)
	}
}

func TestStore_LoadCorrupt_ReturnsDefaultWithError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "intent.yaml")
	if err := os.WriteFile(path, []byte("{{{{not yaml"), 0644); err != nil {
		t.Fatal(err)
	}
	s := intent.NewStore(path)
	f, err := s.Load()
	if err == nil {
		t.Error("expected error for corrupt YAML")
	}
	if _, ok := f.Themes["open"]; !ok {
		t.Error("corrupt file should still return default as fallback")
	}
}
