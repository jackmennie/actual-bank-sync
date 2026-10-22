package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type AccountsConfig struct {
	Mappings []AccountMapping `yaml:"mappings"`
}

type AccountMapping struct {
	Provider        string `yaml:"provider"`
	BankAccountID   string `yaml:"bankAccountId"`
	ActualAccountID string `yaml:"actualAccountId"`
}

func NewAccountsConfig() *AccountsConfig {
	return &AccountsConfig{Mappings: []AccountMapping{}}
}

func LoadAccounts(path string) (*AccountsConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading accounts file: %w", err)
	}

	var config AccountsConfig

	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing accounts file: %w", err)
	}

	if config.Mappings == nil {
		config.Mappings = []AccountMapping{}
	}

	return &config, nil
}

func SaveAccounts(path string, config *AccountsConfig) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("encoding accounts file: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("creating accounts directory: %w", err)
	}
	temporaryPath := path + ".tmp"
	if err := os.WriteFile(temporaryPath, data, 0600); err != nil {
		return fmt.Errorf("writing temporary accounts file: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replacing accounts file: %w", err)
	}

	return nil
}
