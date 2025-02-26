package resultsreceiver

import "time"

type Step struct {
	ExternalID  string     `json:"external_id"`
	Number      int        `json:"number"`
	Name        string     `json:"name"`
	Status      Status     `json:"status"`
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
	Conclusion  Conclusion `json:"conclusion"`
}

type Status int8

const (
	StatusInProgress = 3
	StatusPending    = 5
	StatusCompleted  = 6
)

type Conclusion int8

const (
	ConclusionSuccess   Conclusion = 2
	ConclusionFailure   Conclusion = 3
	ConclusionCancelled Conclusion = 4
	ConclusionSkipped   Conclusion = 7
)
