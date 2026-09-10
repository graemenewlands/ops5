package model

// ActionType represents the type of an OPS5 RHS action.
type ActionType int

const (
	ActionMake ActionType = iota
	ActionModify
	ActionRemove
	ActionWrite
	ActionHalt
	ActionBind
	ActionCBind
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

// WriteArgType represents the type of argument to a write action.
type WriteArgType int

const (
	WriteArgValue WriteArgType = iota
	WriteArgCRLF
	WriteArgTabTo
)

// WriteArg represents an element within a (write ...) action.
type WriteArg struct {
	Type  WriteArgType
	Value Value // value to emit, or column target for tabto
}

// WriteValue creates a WriteArg for a value/literal/variable.
func WriteValue(v Value) WriteArg {
	return WriteArg{Type: WriteArgValue, Value: v}
}

// WriteCRLF creates a WriteArg representing a (crlf) newline directive.
func WriteCRLF() WriteArg {
	return WriteArg{Type: WriteArgCRLF}
}

// WriteTabTo creates a WriteArg representing a (tabto N) column alignment directive.
func WriteTabTo(col Value) WriteArg {
	return WriteArg{Type: WriteArgTabTo, Value: col}
}

// WriteAction writes values/strings/variable bindings and formatting directives to engine output.
type WriteAction struct {
	Args []WriteArg
}

func (a WriteAction) Type() ActionType { return ActionWrite }

// HaltAction instructs the runtime to halt rule execution.
type HaltAction struct{}

func (a HaltAction) Type() ActionType { return ActionHalt }

// BindAction assigns the result of a value or computation to a local variable.
type BindAction struct {
	Variable string // e.g. "<total>" or "total"
	Value    Value  // resolved value, vector, or TypeCompute
}

func (a BindAction) Type() ActionType { return ActionBind }

// CBindAction binds the last element added to working memory (by make, modify, or call) to an element variable.
type CBindAction struct {
	Variable string // e.g. "<p>" or "p"
}

func (a CBindAction) Type() ActionType { return ActionCBind }

// CustomAction executes an arbitrary user function during firing.
type CustomAction struct {
	Name    string
	Execute func(ctx any) error
}

func (a CustomAction) Type() ActionType { return ActionCustom }
