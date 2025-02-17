package ghactions

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
)

func (r *Repository) RegisterAgent(ctx context.Context, name string, label string, publicKey *rsa.PublicKey, token string) (*Response, error) {
	ctx, span := r.tracer.Start(ctx, "RegisterAgent")
	defer span.End()

	requestBody, err := getRegisterAgentRequestBody(name, label, publicKey)
	if err != nil {
		return nil, fmt.Errorf("ghactions.RegisterAgent marshal request body: %w", err)
	}

	url := "/_apis/distributedtask/pools/1/agents"

	headers := http.Header{}

	fmt.Println("setting header")
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	headers.Set("Accept", acceptJSON_60P2)
	headers.Set("Content-Type", contentTypeJSON_60P2)

	response, err := r.doRequest(ctx, "POST", url, nil, headers, requestBody)
	if err != nil {
		return nil, fmt.Errorf("ghactions.RegisterAgent do request: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ghactions.RegisterAgent unexpected status code: %d", response.StatusCode)
	}

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("ghactions.RegisterAgent read response body: %w", err)
	}

	var parsed Response
	err = json.Unmarshal(responseBody, &parsed)
	if err != nil {
		return nil, fmt.Errorf("ghactions.RegisterAgent unmarshal response body: %w", err)
	}

	return &parsed, nil
}

func getRegisterAgentRequestBody(name, label string, publicKey *rsa.PublicKey) (io.Reader, error) {
	request := Request{
		Labels: []Label{
			{
				Name: label,
				Type: "user",
			},
		},
		Authorization: Authorization{
			PublicKey: PublicKey{
				Exponent: base64.StdEncoding.EncodeToString(big.NewInt(int64(publicKey.E)).Bytes()),
				Modulus:  base64.StdEncoding.EncodeToString(publicKey.N.Bytes()),
			},
		},
		Name:              name,
		Version:           "2.322.0",            // TODO: "NOT SET",
		OSDescription:     "Ubuntu 22.04.5 LTS", // TODO: "NOT SET",
		Ephemeral:         false,
		DisableUpdate:     false, // TODO: change to true
		Status:            0,
		ProvisioningState: "Provisioned",

		ID:             0,
		CreatedOn:      "0001-01-01T00:00:00",
		MaxParallelism: 1,
	}

	data, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal request body: %w", err)
	}

	return bytes.NewReader(data), nil
}

type Label struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type Authorization struct {
	PublicKey PublicKey `json:"publicKey"`
}

type PublicKey struct {
	Exponent string `json:"exponent"`
	Modulus  string `json:"modulus"`
}

type Request struct {
	Labels            []Label       `json:"labels"`
	Authorization     Authorization `json:"authorization"`
	Name              string        `json:"name"`
	Version           string        `json:"version"`
	OSDescription     string        `json:"osDescription"`
	Ephemeral         bool          `json:"ephemeral"`
	DisableUpdate     bool          `json:"disableUpdate"`
	Status            int           `json:"status"`
	ProvisioningState string        `json:"provisioningState"`
	ID                int           `json:"id"`
	CreatedOn         string        `json:"createdOn"`
	MaxParallelism    int           `json:"maxParallelism"`
}

type ServerAuthorization struct {
	AuthorizationURL string `json:"authorizationUrl"`
	ClientID         string `json:"clientId"`
}

type Response struct {
	ID              int64               `json:"id"`
	Name            string              `json:"name"`
	RunnerGroupID   int64               `json:"runnerGroupId"`
	RunnerGroupName *string             `json:"runnerGroupName"`
	Authorization   ServerAuthorization `json:"authorization"`
}
