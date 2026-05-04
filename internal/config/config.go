package config

import (
	"encoding/json"
	"os"
)

type Provider string

const (
	ProviderDeepseek   Provider = "deepseek"
	ProviderAnthropic  Provider = "anthropic"
	ProviderOpenAI     Provider = "openai"
	ProviderOpenRouter Provider = "openrouter"
)

type Config struct {
	Provider          Provider          `json:"provider"`
	Model             string            `json:"model"`
	CMModel           string            `json:"cmModel"`
	ImplementerModel  string            `json:"implementerModel"`
	APIKey            string            `json:"apiKey"`
	Keys              map[string]string `json:"keys"`
	CMCount           int               `json:"cmCount"`
	MaxAgents         int               `json:"maxAgents"`
	AgentTimeout      int               `json:"agentTimeout"`
	MaxTokens         int               `json:"maxTokens"`
	EnableTemperature bool              `json:"enableTemperature"`
	Temperature       float64           `json:"temperature"`
	EnableFinalizer   bool              `json:"enableFinalizer"`

	AutoCreateBranch bool   `json:"autoCreateBranch"`
	RequireCleanTree bool   `json:"requireCleanTree"`
	AutoCommit       bool   `json:"autoCommit"`
	BranchPrefix     string `json:"branchPrefix"`
	GitPath          string `json:"gitPath"`
	LastWorkDir      string `json:"lastWorkDir"`

	ShowTimestamps bool   `json:"showTimestamps"`
	AutoScroll     bool   `json:"autoScroll"`
	Theme          string `json:"theme"`
}

func Default() Config {
	return Config{
		Provider:          ProviderDeepseek,
		Model:             "deepseek-v4-flash",
		CMModel:           "deepseek-v4-pro",
		ImplementerModel:  "deepseek-v4-flash",
		Keys:              make(map[string]string),
		CMCount:           1,
		MaxAgents:         4,
		AgentTimeout:      1200,
		MaxTokens:         384000,
		EnableTemperature: false,
		Temperature:       0.3,
		EnableFinalizer:   true,
		AutoCreateBranch:  true,
		AutoCommit:        false,
		BranchPrefix:      "codedone/work-",
		ShowTimestamps:    true,
		AutoScroll:        true,
		Theme:             "dark",
	}
}

func Load(path string) (Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return Config{}, err
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	if cfg.Keys == nil {
		cfg.Keys = make(map[string]string)
	}

	return cfg, nil
}

func Save(path string, cfg Config) error {
	if cfg.Keys == nil {
		cfg.Keys = make(map[string]string)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}
