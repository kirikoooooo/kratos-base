package conf

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureConfigPathBootstrapsDefault(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "configs")

	got, err := EnsureConfigPath(configDir)
	if err != nil {
		t.Fatalf("EnsureConfigPath() error = %v", err)
	}
	if got != configDir {
		t.Fatalf("EnsureConfigPath() = %q, want %q", got, configDir)
	}

	configFile := filepath.Join(configDir, "config.yaml")
	data, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty default config")
	}

	gotAgain, err := EnsureConfigPath(configDir)
	if err != nil {
		t.Fatalf("EnsureConfigPath() second call error = %v", err)
	}
	if gotAgain != configDir {
		t.Fatalf("second call = %q", gotAgain)
	}
}

func TestEnsureConfigPathEmptyUsesWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	got, err := EnsureConfigPath("")
	if err != nil {
		t.Fatalf("EnsureConfigPath() error = %v", err)
	}
	want := filepath.Join(root, "configs")
	if got != want {
		t.Fatalf("EnsureConfigPath() = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(want, "config.yaml")); err != nil {
		t.Fatalf("config.yaml missing: %v", err)
	}
	for _, name := range []string{"model.conf", "policy.csv"} {
		if _, err := os.Stat(filepath.Join(want, "casbin", name)); err != nil {
			t.Fatalf("Casbin %s missing: %v", name, err)
		}
	}
}
