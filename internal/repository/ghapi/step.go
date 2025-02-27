package ghapi

type StepReference struct {
	Type string `json:"type"`
}

type Step struct {
	Type        string        `json:"type"`
	Reference   StepReference `json:"reference"`
	ID          string        `json:"id"`
	ContextName string        `json:"contextName"` // TODO: rename to RefName?
	DisplayName string        `json:"name"`
}
