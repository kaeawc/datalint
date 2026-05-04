package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kaeawc/datalint/internal/config"
)

func TestLoad_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "datalint.yml")
	body := `rules:
  enum-drift:
    lock_in_rows: 50
    max_distinct: 20
  train-eval-overlap:
    prompt_field: input
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	enum := cfg.Rule("enum-drift")
	if got := enum.Int("lock_in_rows", -1); got != 50 {
		t.Errorf("lock_in_rows = %d, want 50", got)
	}
	if got := enum.Int("max_distinct", -1); got != 20 {
		t.Errorf("max_distinct = %d, want 20", got)
	}
	leak := cfg.Rule("train-eval-overlap")
	if got := leak.String("prompt_field", "?"); got != "input" {
		t.Errorf("prompt_field = %q, want input", got)
	}
}

func TestLoad_FileMissing(t *testing.T) {
	_, err := config.Load("/nonexistent-datalint-config-for-test.yml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoad_EmptyYAMLDocument(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "datalint.yml")
	if err := os.WriteFile(path, []byte("# just a comment\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Rules == nil {
		t.Fatal("Rules should be non-nil after Load even on empty doc")
	}
	if got := cfg.Rule("anything").Int("x", 42); got != 42 {
		t.Errorf("missing rule should fall back to default; got %d", got)
	}
}

func TestRuleConfig_Defaults(t *testing.T) {
	cfg := config.Default()
	r := cfg.Rule("nope")
	if got := r.Int("missing", 7); got != 7 {
		t.Errorf("Int default = %d, want 7", got)
	}
	if got := r.String("missing", "x"); got != "x" {
		t.Errorf("String default = %q, want x", got)
	}
}

func TestRuleConfig_WrongType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "datalint.yml")
	body := `rules:
  r:
    n: "not an int"
    s: 42
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r := cfg.Rule("r")
	if got := r.Int("n", 99); got != 99 {
		t.Errorf("string-as-int should default; got %d", got)
	}
	if got := r.String("s", "fallback"); got != "fallback" {
		t.Errorf("int-as-string should default; got %q", got)
	}
}

func TestLoadDiscovered_Miss(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cfg, err := config.LoadDiscovered()
	if err != nil {
		t.Fatalf("LoadDiscovered: %v", err)
	}
	if got := cfg.Rule("x").Int("y", 11); got != 11 {
		t.Errorf("expected default config; got %d", got)
	}
}

func TestLoadDiscovered_Hit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "datalint.yml")
	body := "rules:\n  enum-drift:\n    lock_in_rows: 99\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	cfg, err := config.LoadDiscovered()
	if err != nil {
		t.Fatalf("LoadDiscovered: %v", err)
	}
	if got := cfg.Rule("enum-drift").Int("lock_in_rows", -1); got != 99 {
		t.Errorf("lock_in_rows = %d, want 99", got)
	}
}
