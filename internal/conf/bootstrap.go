package conf

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed default_config.yaml
var defaultConfigYAML []byte

//go:embed default_casbin_model.conf
var defaultCasbinModel []byte

//go:embed default_casbin_policy.csv
var defaultCasbinPolicy []byte

// EnsureConfigPath resolves the config directory/file and materializes a default
// config.yaml when missing (for standalone exe distribution).
func EnsureConfigPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = detectConfigDir()
	}

	info, err := os.Stat(path)
	switch {
	case err == nil && !info.IsDir():
		return path, nil
	case err != nil && !os.IsNotExist(err):
		return "", fmt.Errorf("stat config path: %w", err)
	}

	configDir := path
	configFile := filepath.Join(configDir, "config.yaml")
	if _, err := os.Stat(configFile); err != nil {
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("stat config file: %w", err)
		}
		if err := bootstrapConfigDir(configDir); err != nil {
			return "", err
		}
	}
	return configDir, nil
}

func detectConfigDir() string {
	for _, dir := range configDirCandidates() {
		if configFileExists(dir) {
			return dir
		}
	}
	if wd, err := os.Getwd(); err == nil {
		return filepath.Join(wd, "configs")
	}
	candidates := configDirCandidates()
	if len(candidates) > 0 {
		return candidates[0]
	}
	return "configs"
}

func configDirCandidates() []string {
	candidates := make([]string, 0, 2)
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		if resolved, err := filepath.EvalSymlinks(exeDir); err == nil {
			exeDir = resolved
		}
		if !isEphemeralExecutableDir(exeDir) {
			candidates = append(candidates, filepath.Join(exeDir, "configs"))
		}
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "configs"))
	}
	return candidates
}

func configFileExists(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "config.yaml"))
	return err == nil
}

func isEphemeralExecutableDir(exeDir string) bool {
	return strings.Contains(exeDir, "/go-build") || strings.Contains(exeDir, "\\go-build")
}

func bootstrapConfigDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	configFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configFile, defaultConfigYAML, 0o644); err != nil {
		return fmt.Errorf("write default config: %w", err)
	}
	casbinDir := filepath.Join(dir, "casbin")
	if err := os.MkdirAll(casbinDir, 0o755); err != nil {
		return fmt.Errorf("create Casbin config directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(casbinDir, "model.conf"), defaultCasbinModel, 0o644); err != nil {
		return fmt.Errorf("write Casbin model: %w", err)
	}
	if err := os.WriteFile(filepath.Join(casbinDir, "policy.csv"), defaultCasbinPolicy, 0o644); err != nil {
		return fmt.Errorf("write Casbin policy: %w", err)
	}
	return nil
}

// ResolveFilePaths resolves Casbin assets against the configuration directory.
func ResolveFilePaths(configDir string, security *Security) *Security {
	if security == nil {
		return nil
	}
	if file := strings.TrimSpace(security.GetCasbinModelFile()); file != "" && !filepath.IsAbs(file) {
		security.CasbinModelFile = filepath.Join(configDir, file)
	}
	if file := strings.TrimSpace(security.GetCasbinPolicyFile()); file != "" && !filepath.IsAbs(file) {
		security.CasbinPolicyFile = filepath.Join(configDir, file)
	}
	return security
}
