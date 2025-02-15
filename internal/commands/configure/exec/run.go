package exec

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"os"

	"github.com/artuross/github-actions-runner/internal/repository/ghactions"
	"github.com/artuross/github-actions-runner/internal/repository/ghrest"
	"github.com/artuross/github-actions-runner/internal/repository/ghrunner"
	"go.opentelemetry.io/otel/trace"
)

type Config struct {
	RunnerConfigFilePath string
	PrivateKeyFilePath   string
	TargetType           string
	TargetPath           string
	RunnerName           string
	RunnerLabel          string
}

type Executor struct {
	actionsClient *ghactions.Repository
	restClient    *ghrest.Repository
	runnerClient  *ghrunner.Repository
	tracer        trace.Tracer
}

func NewExecutor(
	actionsClient *ghactions.Repository,
	restClient *ghrest.Repository,
	runnerClient *ghrunner.Repository,
	traceProvider trace.TracerProvider,
) *Executor {
	return &Executor{
		actionsClient: actionsClient,
		restClient:    restClient,
		runnerClient:  runnerClient,
		tracer:        traceProvider.Tracer("github.com/artuross/internal/commands/configure/exec"),
	}
}

func (e *Executor) Run(ctx context.Context, config Config) error {
	ctx, span := e.tracer.Start(ctx, "run")
	defer span.End()

	// TODO: rand.Reader must come from params
	privateKey, publicKey, err := generateKeyPair(rand.Reader)
	if err != nil {
		return fmt.Errorf("generate RSA key: %w", err)
	}

	runnerConfig, err := e.registerRunner(ctx, config, publicKey)
	if err != nil {
		return fmt.Errorf("register runner: %w", err)
	}

	if err := saveConfigFile(config.RunnerConfigFilePath, runnerConfig); err != nil {
		return fmt.Errorf("save config file: %w", err)
	}

	if err := savePrivateKeyFile(config.PrivateKeyFilePath, privateKey); err != nil {
		return fmt.Errorf("save private key file: %w", err)
	}

	return nil
}

func (e *Executor) fetchRegistrationToken(ctx context.Context, targetType string, targetPath string) (string, error) {
	if targetType == "organization" {
		token, err := e.restClient.CreateOrganizationRunnerRegistrationToken(ctx, targetPath)
		if err != nil {
			return "", fmt.Errorf("create organization runner registration token: %w", err)
		}

		return token.Token, nil
	}

	return "", fmt.Errorf("unsupported target type: %s", targetType)
}

// use https://github.com/microsoft/azure-devops-go-api/blob/dev/azuredevops/taskagent/client.go
// https://learn.microsoft.com/en-us/rest/api/azure/devops/distributedtask/agentclouds/list?view=azure-devops-rest-7.2
func (e *Executor) registerRunner(ctx context.Context, cfg Config, publicKey *rsa.PublicKey) (*RunnerConfig, error) {
	registrationToken, err := e.fetchRegistrationToken(ctx, cfg.TargetType, cfg.TargetPath)
	if err != nil {
		return nil, fmt.Errorf("create organization runner registration token: %w", err)
	}

	shardResponse, err := e.runnerClient.Register(ctx, cfg.TargetPath, registrationToken)
	if err != nil {
		return nil, fmt.Errorf("register runner: %w", err)
	}

	e.actionsClient = e.actionsClient.WithBaseURL(shardResponse.Url)

	agentResponse, err := e.actionsClient.RegisterAgent(ctx, cfg.RunnerName, cfg.RunnerLabel, publicKey, shardResponse.Token)
	if err != nil {
		return nil, fmt.Errorf("register agent: %w", err)
	}

	runnerConfig := RunnerConfig{
		AuthClientID:    agentResponse.Authorization.ClientID,
		AuthURL:         agentResponse.Authorization.AuthorizationURL,
		RunnerGroupID:   agentResponse.RunnerGroupID,
		RunnerGroupName: agentResponse.RunnerGroupName,
		RunnerID:        agentResponse.ID,
		RunnerName:      agentResponse.Name,
		ShardURL:        shardResponse.Url,
	}

	return &runnerConfig, nil
}

func generateKeyPair(random io.Reader) (*rsa.PrivateKey, *rsa.PublicKey, error) {
	privateKey, err := rsa.GenerateKey(random, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("generate RSA key: %w", err)
	}

	publicKey := privateKey.Public().(*rsa.PublicKey)

	return privateKey, publicKey, nil
}

func saveConfigFile(path string, config *RunnerConfig) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal runner config file: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("save runner config file: %w", err)
	}

	return nil
}

func savePrivateKeyFile(path string, key *rsa.PrivateKey) error {
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

type RunnerConfig struct {
	AuthClientID    string  `json:"authClientId"`
	AuthURL         string  `json:"authUrl"`
	RunnerGroupID   int64   `json:"runnerGroupID"`
	RunnerGroupName *string `json:"runnerGroupName"`
	RunnerID        int64   `json:"runnerId"`
	RunnerName      string  `json:"runnerName"`
	ShardURL        string  `json:"shardUrl"`
}
