package step

var _ Step = (*Noop)(nil)

type Noop struct {
	id          string
	parentID    *string
	displayName string
	refName     string
}

// for Step
func (s *Noop) ID() string          { return s.id }
func (s *Noop) ParentID() *string   { return s.parentID }
func (s *Noop) DisplayName() string { return s.displayName }
func (s *Noop) RefName() string     { return s.refName }

// for Typer
func (s *Noop) Type() string { return "task" } // TODO: cleanup
