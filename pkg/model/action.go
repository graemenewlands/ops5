package model

// ActionType represents the type of an OPS5 RHS action.
type ActionType int

const (
	ActionMake ActionType = iota
	ActionModify
	ActionRemove
	ActionWrite
	ActionHalt
	ActionCustom
)

// Action represents a RHS action to be executed when a rule fires.
type Action interface {
	Type() ActionType
}

// ActionMake creates a new WME.
type MakeAction struct {
	Class      string
	Attributes map[string]Value
}

func (a MakeAction) Type() ActionType { return ActionMake }

// ModifyAction modifies an existing WME identified either by element variable or by 1-based CE index.
type ModifyAction struct {
	TargetElementVar string // e.g. "g" if `<g>`
	TargetIndex      int    // 1-based CE index if specified, otherwise 0
	Attributes       map[string]Value
}

func (a ModifyAction) Type() ActionType { return ActionModify }

// RemoveAction removes an existing WME identified by element variable or 1-based index.
type RemoveAction struct {
	TargetElementVar string
	TargetIndex      int
}

func (a RemoveAction) Type() ActionType { return ActionRemove }

// WriteAction writes values/strings/variable bindings to engine output.
type WriteAction struct {
	Items []Value
}

func (a WriteAction) Type() ActionType { return ActionWrite }

// HaltAction instructs the runtime to halt rule execution.
type HaltAction struct{}

func (a HaltAction) Type() ActionType { return ActionHalt }

// CustomAction executes an arbitrary user function during firing.
type CustomAction struct {
	Name    string
	Execute func(ctx any) error
}

func (a CustomAction) Type() ActionType { return ActionCustom }
