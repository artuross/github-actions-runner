package config

import (
	"fmt"
	"strings"
)

type Flagger interface {
	String(name string) string
	Int(name string) int
}

type Config struct {
	GithubToken          string
	PrivateKeyFilePath   string
	RunnerConfigFilePath string
	RunnerLabel          string
	RunnerName           string
	TargetPath           string
	TargetType           string
}

func Read(flags Flagger, getEnv func(string) string) (*Config, error) {
	// envs - required
	githubToken := getEnv("GITHUB_API_TOKEN")
	if githubToken == "" {
		return nil, fmt.Errorf("env var GITHUB_API_TOKEN is required")
	}

	// flags - required
	runnerName := flags.String("name")
	if runnerName == "" {
		return nil, fmt.Errorf("flag --name is required")
	}

	runnerLabel := flags.String("label")
	if runnerLabel == "" {
		return nil, fmt.Errorf("flag --label is required")
	}

	organization := flags.String("organization")
	if organization == "" {
		return nil, fmt.Errorf("flag --organization is required")
	}

	targetType := "organization"
	targetPath := organization

	targetPath = strings.TrimPrefix(targetPath, "https://github.com/")

	// flags - optional
	privateKeyFile := flags.String("private-key-file")
	runnerConfigFile := flags.String("runner-config-file")

	cfg := Config{
		GithubToken:          githubToken,
		PrivateKeyFilePath:   privateKeyFile,
		RunnerConfigFilePath: runnerConfigFile,
		RunnerLabel:          runnerLabel,
		RunnerName:           runnerName,
		TargetPath:           targetPath,
		TargetType:           targetType,
	}

	return &cfg, nil
}

func Print(cfg *Config) {
	fmt.Println("Running with config:")
	fmt.Printf("  Name: %s\n", cfg.RunnerName)
	fmt.Printf("  Label: %s\n", cfg.RunnerLabel)
	fmt.Printf("  Target Type: %s\n", cfg.TargetType)
	fmt.Printf("  Target Path: %s\n", cfg.TargetPath)
	fmt.Printf("  Runner Config File: %s\n", cfg.RunnerConfigFilePath)
	fmt.Printf("  Private Key File: %s\n", cfg.PrivateKeyFilePath)
}
