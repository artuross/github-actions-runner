package runnerconfig

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
)

type Config struct {
	AuthClientID    string  `json:"authClientId"`
	AuthURL         string  `json:"authUrl"`
	RunnerGroupID   int64   `json:"runnerGroupID"`
	RunnerGroupName *string `json:"runnerGroupName"`
	RunnerID        int64   `json:"runnerId"`
	RunnerName      string  `json:"runnerName"`
	ShardURL        string  `json:"shardUrl"`
}

func SaveConfigFile(path string, config *Config) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal runner config file: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("save runner config file: %w", err)
	}

	return nil
}

func SavePrivateKeyFile(path string, key *rsa.PrivateKey) error {
	pemBlock := pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}

	data := pem.EncodeToMemory(&pemBlock)

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("save private key file: %w", err)
	}

	return nil
}

func ReadConfigFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read runner config file: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("unmarshal runner config file: %w", err)
	}

	return &config, nil
}

func ReadPrivateKeyFile(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read private key file: %w", err)
	}

	block, _ := pem.Decode(data)
	if block == nil || block.Type != "RSA PRIVATE KEY" {
		return nil, fmt.Errorf("invalid private key file")
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	return key, nil
}
