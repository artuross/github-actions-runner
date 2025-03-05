package semconv

// Runner
const (
	// Numeric ID of the runner as registered with Github API.
	RunnerID = "runner_id"
)

const (
	// ID of the message sent by the broker.
	MessageID = "message_id"

	// Unique ID for the session. A runner may have only 1 active session.
	SessionID = "session_id"
)

// Workflow, Job & Step
const (
	// Stable ID for the step. The same job executed twice will have the same ID.
	// Note that this ID is only stable for steps defined in the YAML file. Generated steps
	// (pre, post, from composite actions) will have a unique ID each time.
	StepID = "step_id"
)
