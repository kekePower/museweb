package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds the application configuration
type Config struct {
	Server struct {
		Address    string `yaml:"address"`
		Port       string `yaml:"port"`
		PromptsDir string `yaml:"prompts_dir"`
		Debug      bool   `yaml:"debug"`
	} `yaml:"server"`
	Model struct {
		Backend string `yaml:"backend"`
		Name    string `yaml:"name"`
		// DisableThinking disables the thinking tag for DeepSeek and r1-1776 models
		DisableThinking bool `yaml:"disable_thinking"`
	} `yaml:"model"`
	OpenAI struct {
		APIKey  string `yaml:"api_key"`
		APIBase string `yaml:"api_base"`
	} `yaml:"openai"`
	Ollama struct {
		APIKey  string `yaml:"api_key"`
		APIBase string `yaml:"api_base"`
	} `yaml:"ollama"`
}

// Load reads the configuration from a YAML file
func Load(path string) (*Config, error) {
	var cfg Config

	// Set default values
	cfg.Server.Address = "127.0.0.1"
	cfg.Server.Port = "8080"
	cfg.Server.PromptsDir = "prompts"
	cfg.Model.Backend = "ollama"
	cfg.Model.Name = "llama3"
	cfg.Ollama.APIBase = "http://localhost:11434"

	// Read the config file
	data, err := os.ReadFile(path)
	if err != nil {
		return &cfg, err
	}

	// Parse the YAML
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return &cfg, err
	}

	return &cfg, nil
}
