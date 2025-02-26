package api

import (
	"encoding/json"
	"errors"

	"github.com/artuross/github-actions-runner/internal/repository/ghapi"
)

type Wrapper struct {
	Message any
}

func (w *Wrapper) UnmarshalJSON(data []byte) error {
	type internalWrapper struct {
		MessageType string `json:"messageType"`
	}

	var wrapper internalWrapper
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return err
	}

	switch wrapper.MessageType {
	case ghapi.MessageTypeBrokerMigration:
		type internalBrokerMigrationWrapper struct {
			MessageType string `json:"messageType"`
			Body        string `json:"body"`
		}

		var wrapper internalBrokerMigrationWrapper
		if err := json.Unmarshal(data, &wrapper); err != nil {
			return err
		}

		type internalMessageBrokerMigration struct {
			BrokerBaseURL string `json:"brokerBaseUrl"`
		}

		var message internalMessageBrokerMigration
		if err := json.Unmarshal([]byte(wrapper.Body), &message); err != nil {
			return err
		}

		w.Message = ghapi.BrokerMigration{
			BaseURL: message.BrokerBaseURL,
		}

		return nil

	case ghapi.MessageTypeRunnerJobRequest:
		type internalBrokerMessage struct {
			MessageType string `json:"messageType"`
			MessageID   int64  `json:"messageId"`
			Body        string `json:"body"`
		}

		var wrapper internalBrokerMessage
		if err := json.Unmarshal(data, &wrapper); err != nil {
			return err
		}

		type internalMessageRunnerJobRequest struct {
			RunnerRequestID string `json:"runner_request_id"`
			RunServiceURL   string `json:"run_service_url"`
			BillingOwnerID  string `json:"billing_owner_id"`
		}

		var message internalMessageRunnerJobRequest
		if err := json.Unmarshal([]byte(wrapper.Body), &message); err != nil {
			return err
		}

		w.Message = ghapi.BrokerMessage{
			MessageID: wrapper.MessageID,
			Message: ghapi.MessageRunnerJobRequest{
				RunnerRequestID: message.RunnerRequestID,
				RunServiceURL:   message.RunServiceURL,
				BillingOwnerID:  message.BillingOwnerID,
			},
		}

		return nil

	case ghapi.MessageTypePipelineAgentJobRequest:
		type (
			internalStepReference struct {
				Type string `json:"type"`
			}

			internalStep struct {
				Type        string                `json:"type"`
				Reference   internalStepReference `json:"reference"`
				Id          string                `json:"id"`
				ContextName string                `json:"contextName"`
				Name        string                `json:"name"`
			}

			internalPlan struct {
				PlanID string `json:"planId"`
			}

			internalTimeline struct {
				ID string `json:"id"`
			}

			internalParameters struct {
				AccessToken string `json:"AccessToken"`
			}

			internalAuthorization struct {
				Parameters internalParameters `json:"parameters"`
			}

			internalData struct {
				ResultsServiceURL string `json:"ResultsServiceUrl"`
			}

			internalEndpoint struct {
				Authorization internalAuthorization `json:"authorization"`
				Data          internalData          `json:"data"`
				URL           string                `json:"url"`
			}

			internalResources struct {
				Endpoints []internalEndpoint `json:"endpoints"`
			}

			internalPipelineAgentJobRequest struct {
				MessageType    string            `json:"messageType"`
				JobID          string            `json:"jobId"`
				JobDisplayName string            `json:"jobDisplayName"`
				JobName        string            `json:"jobName"`
				RequestID      int64             `json:"requestId"`
				Steps          []internalStep    `json:"steps"`
				Plan           internalPlan      `json:"plan"`
				Timeline       internalTimeline  `json:"timeline"`
				Resources      internalResources `json:"resources"`
			}
		)

		convertToSteps := func(internalSteps []internalStep) []ghapi.Step {
			steps := make([]ghapi.Step, 0, len(internalSteps))

			for _, is := range internalSteps {
				reference := ghapi.StepReference{
					Type: is.Reference.Type,
				}

				step := ghapi.Step{
					Type:        is.Type,
					Reference:   reference,
					ID:          is.Id,
					ContextName: is.ContextName,
					Name:        is.Name,
				}

				steps = append(steps, step)
			}

			return steps
		}

		convertToEndpoints := func(internalEndpoints []internalEndpoint) []ghapi.Endpoint {
			endpoints := make([]ghapi.Endpoint, 0, len(internalEndpoints))

			for _, ie := range internalEndpoints {
				endpoint := ghapi.Endpoint{
					Authorization: ghapi.EndpointAuthorization{
						Parameters: ghapi.EndpointAuthorizationParameters{
							AccessToken: ie.Authorization.Parameters.AccessToken,
						},
					},
					ActionsServiceURL: ie.URL,
					ResultsServiceURL: ie.Data.ResultsServiceURL,
				}

				endpoints = append(endpoints, endpoint)
			}

			return endpoints
		}

		var message internalPipelineAgentJobRequest
		if err := json.Unmarshal(data, &message); err != nil {
			return err
		}

		w.Message = ghapi.PipelineAgentJobRequest{
			JobID:          message.JobID,
			JobDisplayName: message.JobDisplayName,
			JobName:        message.JobName,
			RequestID:      message.RequestID,
			Steps:          convertToSteps(message.Steps),
			Plan: ghapi.Plan{
				PlanID: message.Plan.PlanID,
			},
			Timeline: ghapi.Timeline{
				ID: message.Timeline.ID,
			},
			Resources: ghapi.Resources{
				Endpoints: convertToEndpoints(message.Resources.Endpoints),
			},
		}

		return nil
	}

	return errors.New("unknown message type - likely forgot to return")
}
