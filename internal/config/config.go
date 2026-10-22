package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Actual              ActualConfig    `yaml:"actual"`
	Providers           ProvidersConfig `yaml:"providers"`
	MappingsFile        string          `yaml:"mappingsFile"`
	SyncStateFile       string          `yaml:"syncStateFile"`
	InitialLookbackDays int             `yaml:"initialLookbackDays"`
	Verbose             bool            `yaml:"verbose"`
}

type ActualConfig struct {
	Password      string `yaml:"password"`
	EncryptionKey string `yaml:"encryptionKey"`
	BaseURL       string `yaml:"baseUrl"`
	SyncID        string `yaml:"syncId"`
	DataDir       string `yaml:"dataDir"`
	NodeBinary    string `yaml:"nodeBinary"`
	HelperPath    string `yaml:"helperPath"`
}

type ProvidersConfig struct {
	UP UPConfig `yaml:"up"`
}

type UPConfig struct {
	APIKey string `yaml:"apiKey"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var config Config

	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}

	return &config, nil
}

func (c *Config) Validate() error {
	if c.MappingsFile == "" {
		c.MappingsFile = "accounts.yaml"
	}
	if c.SyncStateFile == "" {
		c.SyncStateFile = "sync-state.yaml"
	}
	if c.InitialLookbackDays == 0 {
		c.InitialLookbackDays = 90
	}
	if c.InitialLookbackDays < 0 {
		return fmt.Errorf("initial lookback days cannot be negative")
	}
	if c.Actual.BaseURL == "" {
		return fmt.Errorf("Actual server base URL is required")
	}
	if c.Actual.Password == "" {
		return fmt.Errorf("Actual server password is required")
	}
	if c.Actual.SyncID == "" {
		return fmt.Errorf("Actual budget sync ID is required")
	}
	if c.Providers.UP.APIKey == "" {
		return fmt.Errorf("UP API key is required")
	}

	return nil
}
