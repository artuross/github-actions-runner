package ghapi

type MessageType string

// TODO: add custom type
const (
	MessageTypeBrokerMigration         = "BrokerMigration"         // server requesting to connect to the broker
	MessageTypePipelineAgentJobRequest = "PipelineAgentJobRequest" // job request details
	MessageTypeRunnerJobRequest        = "RunnerJobRequest"        // job request sent by broker
)

type Message interface {
	MessageType() MessageType
}

type (
	BrokerMessage struct {
		MessageID int64
		Message   any
	}

	BrokerMigration struct {
		BaseURL string
	}

	MessageRunnerJobRequest struct {
		RunnerRequestID string
		RunServiceURL   string
		BillingOwnerID  string
	}

	PipelineAgentJobRequest struct {
		JobID          string
		JobDisplayName string
		JobName        string
		RequestID      int64
		Steps          []Step
		Plan           Plan
		Timeline       Timeline
		Resources      Resources
	}
)

func (BrokerMessage) MessageType() MessageType           { return "BrokerMessage" } // not a message per se, but a message wrapper
func (BrokerMigration) MessageType() MessageType         { return MessageTypeBrokerMigration }
func (MessageRunnerJobRequest) MessageType() MessageType { return MessageTypeRunnerJobRequest }
func (PipelineAgentJobRequest) MessageType() MessageType { return MessageTypePipelineAgentJobRequest }

type Plan struct {
	PlanID string
}

type Timeline struct {
	ID string
}

type EndpointAuthorizationParameters struct {
	AccessToken string
}

type EndpointAuthorization struct {
	Parameters EndpointAuthorizationParameters
}

type Endpoint struct {
	Authorization     EndpointAuthorization
	ActionsServiceURL string
	ResultsServiceURL string
}

type Resources struct {
	Endpoints []Endpoint
}
