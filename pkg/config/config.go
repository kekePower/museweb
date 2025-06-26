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
		// ReasoningModels is a list of model name patterns that support reasoning/thinking tags
		ReasoningModels []string `yaml:"reasoning_models"`
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
	cfg.Model.ReasoningModels = []string{
		// Most specific patterns first (to avoid conflicts)
		"deepseek-r1-distill",           // DeepSeek R1 distilled models (most specific)
		"r1-distill",                    // Other R1 distilled models
		"sonar-reasoning-pro",           // Perplexity Sonar reasoning pro models
		"sonar-reasoning",               // Perplexity Sonar reasoning models
		"gemini-2.5-flash-lite-preview-06-17", // Specific Gemini model
		"gemini-2.5-flash",              // Gemini 2.5 Flash models
		"r1-1776",                       // OpenAI R1-1776 models
		"qwen3",                         // Qwen3 models (specific)
		"deepseek",                      // DeepSeek models (general, after specific)
		"qwen",                          // Qwen models (general, after specific)
	}
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
