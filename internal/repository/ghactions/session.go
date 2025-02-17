package ghactions

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Agent struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Version       string `json:"version"`
	OSDescription string `json:"osDescription"`
	Ephemeral     bool   `json:"ephemeral"`
}

type CreateSessionBody struct {
	SessionID         string `json:"sessionId"`
	OwnerName         string `json:"ownerName"`
	Agent             Agent  `json:"agent"`
	UseFipsEncryption bool   `json:"useFipsEncryption"` // always set to false
}

type CreateSessionSuccess struct {
	SessionID         string
	EncryptionKey     string
	UseFipsEncryption bool
}

func (r *Repository) CreateSession(ctx context.Context, poolID int64) (*CreateSessionSuccess, error) {
	requestBody := CreateSessionBody{
		SessionID: "00000000-0000-0000-0000-000000000000", // TODO: take from params, this appears to always be 0
		OwnerName: "4a4fda292974 (PID: 76113)",            // TODO: take from params
		Agent: Agent{
			ID:            3,                    // take from params
			Name:          "4a4fda292974",       // take from params
			Version:       "2.322.0",            // take from params
			OSDescription: "Ubuntu 22.04.5 LTS", // take from params
			Ephemeral:     false,                // take from params
		},
		UseFipsEncryption: false,
	}

	encodedBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("encode JSON body: %w", err)
	}

	path := fmt.Sprintf("/_apis/distributedtask/pools/%d/sessions", poolID)
	response, err := r.doRequest(ctx, "POST", path, nil, nil, bytes.NewReader(encodedBody))
	if err != nil {
		return nil, fmt.Errorf("do HTTP request: %w", err)
	}

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", response.StatusCode)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	var responseBody createSession200
	if err := json.Unmarshal(body, &responseBody); err != nil {
		return nil, fmt.Errorf("decode JSON body: %w", err)
	}

	respBody := CreateSessionSuccess{
		SessionID:         responseBody.SessionID,
		EncryptionKey:     responseBody.EncryptionKey.Value,
		UseFipsEncryption: responseBody.UseFipsEncryption,
	}

	return &respBody, nil
}

type encryptionKey struct {
	Encrypted bool   `json:"encrypted"`
	Value     string `json:"value"`
}

type createSession200 struct {
	SessionID         string        `json:"sessionId"`
	EncryptionKey     encryptionKey `json:"encryptionKey"`
	UseFipsEncryption bool          `json:"useFipsEncryption"`
}
