package service

import (
	"os"
	"path/filepath"
	"testing"

	"kratos-demo/internal/conf"
)

func TestApplyCLICredentials(t *testing.T) {
	ai := &conf.AI{Openai: &conf.AI_OpenAI{}}
	creds := &CLICredentials{
		APIKey:  "sk-test-key",
		BaseURL: "https://example.com/v1",
	}
	ApplyCLICredentials(ai, creds)
	if ai.Openai.GetApiKey() != creds.APIKey {
		t.Fatalf("api_key = %q", ai.Openai.GetApiKey())
	}
	if ai.Openai.GetBaseUrl() != creds.BaseURL {
		t.Fatalf("base_url = %q", ai.Openai.GetBaseUrl())
	}
}

func TestSaveLoadCLICredentials(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	creds := &CLICredentials{
		APIKey:  "sk-roundtrip",
		BaseURL: "https://proxy.example/v1",
	}
	if err := SaveCLICredentials(creds); err != nil {
		t.Fatalf("SaveCLICredentials() error = %v", err)
	}

	path := filepath.Join(dir, ".myagent", "credentials.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("credentials mode = %o, want 0600", info.Mode().Perm())
	}

	loaded, err := LoadCLICredentials()
	if err != nil {
		t.Fatalf("LoadCLICredentials() error = %v", err)
	}
	if loaded.APIKey != creds.APIKey || loaded.BaseURL != creds.BaseURL {
		t.Fatalf("loaded = %+v, want %+v", loaded, creds)
	}
}

func TestMaskCLIAPIKey(t *testing.T) {
	if got := maskCLIAPIKey(""); got != "(未设置)" {
		t.Fatalf("empty = %q", got)
	}
	if got := maskCLIAPIKey("sk-1234567890abcd"); got != "sk-1*********abcd" {
		t.Fatalf("masked = %q", got)
	}
}
