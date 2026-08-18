// Package config does config things.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Azure        AzureConfig          `yaml:"azure"`
	Sender       string               `yaml:"sender"` // UPN of the sending mailbox e.g. releases@company.com
	Environments map[string]EnvConfig `yaml:"environments"`
}

type AzureConfig struct {
	TenantID     string `yaml:"tenant_id"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"` // can also be set via env var AZURE_CLIENT_SECRET
}

type EnvConfig struct {
	SubjectPrefix     string   `yaml:"subject_prefix"`      // e.g. "[PROD RELEASE]"
	AlwaysIncludeTags []string `yaml:"always_include_tags"` // tags always added for this env
}

// DefaultConfig returns a config with sensible defaults (no Azure credentials).
// Used when running in Outlook mode without a config file.
func DefaultConfig() *Config {
	return &Config{
		Environments: map[string]EnvConfig{
			"production": {SubjectPrefix: "live'i läinud AX2012 uuendused"},
			"staging":    {SubjectPrefix: "[STAGING RELEASE]"},
			"hotfix":     {SubjectPrefix: "🔥 [HOTFIX]"},
		},
	}
}

// LoadMinimal loads config but does NOT fail on missing Azure credentials.
// Used for Outlook COM mode where Azure auth is not needed.
func LoadMinimal(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %q: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config YAML: %w", err)
	}
	return &cfg, nil
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %q: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config YAML: %w", err)
	}

	// Allow client secret via environment variable (better for CI/CD)
	if secret := os.Getenv("AZURE_CLIENT_SECRET"); secret != "" {
		cfg.Azure.ClientSecret = secret
	}

	if cfg.Azure.TenantID == "" {
		return nil, fmt.Errorf("azure.tenant_id is required in config")
	}
	if cfg.Azure.ClientID == "" {
		return nil, fmt.Errorf("azure.client_id is required in config")
	}
	if cfg.Azure.ClientSecret == "" {
		return nil, fmt.Errorf("azure.client_secret is required (config or AZURE_CLIENT_SECRET env var)")
	}
	if cfg.Sender == "" {
		return nil, fmt.Errorf("sender email is required in config")
	}

	return &cfg, nil
}
