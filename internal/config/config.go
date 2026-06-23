package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds all settings from .codereview.yml.
type Config struct {
	Version           int      `yaml:"version"`
	FailOn            string   `yaml:"failOn"`
	OpenAIModel       string   `yaml:"openAIModel"`
	MaxFilesPerReview int      `yaml:"maxFilesPerReview"`
	DiffContext       int      `yaml:"diffContext"`
	ExcludePatterns   []string `yaml:"excludePatterns"`
	Rules             []string `yaml:"rules"`
}

// Load reads and parses the YAML config file at path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	cfg.applyDefaults()
	return &cfg, nil
}

// Default returns a Config with sensible defaults (used when .codereview.yml is absent).
func Default() *Config {
	cfg := &Config{}
	cfg.applyDefaults()
	return cfg
}

func (c *Config) applyDefaults() {
	if c.FailOn == "" {
		c.FailOn = "blocker"
	}
	if c.OpenAIModel == "" {
		c.OpenAIModel = "gpt-4o"
	}
	if c.MaxFilesPerReview == 0 {
		c.MaxFilesPerReview = 20
	}
	if c.DiffContext == 0 {
		c.DiffContext = 3
	}
}
