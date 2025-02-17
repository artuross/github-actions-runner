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
