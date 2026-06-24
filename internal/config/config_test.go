package config

import (
	"errors"
	"os"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.FailOn != "blocker" {
		t.Errorf("FailOn = %q, want %q", cfg.FailOn, "blocker")
	}
	if cfg.OpenAIModel != "gpt-4o" {
		t.Errorf("OpenAIModel = %q, want %q", cfg.OpenAIModel, "gpt-4o")
	}
	if cfg.MaxFilesPerReview != 20 {
		t.Errorf("MaxFilesPerReview = %d, want 20", cfg.MaxFilesPerReview)
	}
	if cfg.DiffContext != 3 {
		t.Errorf("DiffContext = %d, want 3", cfg.DiffContext)
	}
}

func TestLoad_ValidFile(t *testing.T) {
	content := `
version: 1
failOn: major
openAIModel: gpt-4o-mini
maxFilesPerReview: 5
excludePatterns:
  - "**/*_test.go"
rules:
  - "No panic in production code"
  - "Check all errors"
`
	f := writeTempFile(t, content)
	cfg, err := Load(f)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.FailOn != "major" {
		t.Errorf("FailOn = %q, want %q", cfg.FailOn, "major")
	}
	if cfg.OpenAIModel != "gpt-4o-mini" {
		t.Errorf("OpenAIModel = %q, want %q", cfg.OpenAIModel, "gpt-4o-mini")
	}
	if cfg.MaxFilesPerReview != 5 {
		t.Errorf("MaxFilesPerReview = %d, want 5", cfg.MaxFilesPerReview)
	}
	if len(cfg.ExcludePatterns) != 1 {
		t.Errorf("ExcludePatterns len = %d, want 1", len(cfg.ExcludePatterns))
	}
	if len(cfg.Rules) != 2 {
		t.Errorf("Rules len = %d, want 2", len(cfg.Rules))
	}
}

func TestLoad_AppliesDefaultsForMissingFields(t *testing.T) {
	// A minimal config that omits failOn and openAIModel.
	content := "version: 1\n"
	f := writeTempFile(t, content)

	cfg, err := Load(f)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.FailOn != "blocker" {
		t.Errorf("FailOn = %q, want default %q", cfg.FailOn, "blocker")
	}
	if cfg.MaxFilesPerReview != 20 {
		t.Errorf("MaxFilesPerReview = %d, want default 20", cfg.MaxFilesPerReview)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/.codereview.yml")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
	// A missing file must be distinguishable from a malformed one so callers
	// can fall back to defaults only when the file is genuinely absent.
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing file error should wrap os.ErrNotExist, got %v", err)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	f := writeTempFile(t, "failOn: [not a string\n")
	_, err := Load(f)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
	// A malformed config must NOT look like a missing file, otherwise the
	// caller would silently degrade to defaults and drop the user's settings.
	if errors.Is(err, os.ErrNotExist) {
		t.Errorf("malformed config should not report as missing, got %v", err)
	}
}

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "codereview-*.yml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	f.Close()
	return f.Name()
}
