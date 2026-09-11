package model

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

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
	ActionOpenFile
	ActionCloseFile
	ActionDefault
	ActionCustom
)

// Action represents a RHS action to be executed when a rule fires.
type Action interface {
	Type() ActionType
	String() string
}

// ActionMake creates a new WME.
type MakeAction struct {
	Class      string
	Attributes map[string]Value
}

func (a MakeAction) Type() ActionType { return ActionMake }

func (a MakeAction) String() string {
	var b strings.Builder
	b.WriteString("(make ")
	b.WriteString(a.Class)
	keys := make([]string, 0, len(a.Attributes))
	for k := range a.Attributes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString(fmt.Sprintf(" ^%s %s", k, a.Attributes[k].String()))
	}
	b.WriteString(")")
	return b.String()
}

// ModifyAction modifies an existing WME identified either by element variable or by 1-based CE index.
type ModifyAction struct {
	TargetElementVar string // e.g. "g" if `<g>`
	TargetIndex      int    // 1-based CE index if specified, otherwise 0
	Attributes       map[string]Value
}

func (a ModifyAction) Type() ActionType { return ActionModify }

func (a ModifyAction) String() string {
	var b strings.Builder
	b.WriteString("(modify ")
	if a.TargetElementVar != "" {
		b.WriteString(fmt.Sprintf("<%s>", a.TargetElementVar))
	} else {
		b.WriteString(strconv.Itoa(a.TargetIndex))
	}
	keys := make([]string, 0, len(a.Attributes))
	for k := range a.Attributes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString(fmt.Sprintf(" ^%s %s", k, a.Attributes[k].String()))
	}
	b.WriteString(")")
	return b.String()
}

// RemoveAction removes an existing WME identified by element variable or 1-based index.
type RemoveAction struct {
	TargetElementVar string
	TargetIndex      int
}

func (a RemoveAction) Type() ActionType { return ActionRemove }

func (a RemoveAction) String() string {
	if a.TargetElementVar != "" {
		return fmt.Sprintf("(remove <%s>)", a.TargetElementVar)
	}
	return fmt.Sprintf("(remove %d)", a.TargetIndex)
}

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

func (a WriteAction) String() string {
	var b strings.Builder
	b.WriteString("(write")
	for _, arg := range a.Args {
		b.WriteString(" ")
		switch arg.Type {
		case WriteArgCRLF:
			b.WriteString("(crlf)")
		case WriteArgTabTo:
			b.WriteString(fmt.Sprintf("(tabto %s)", arg.Value.String()))
		case WriteArgValue:
			b.WriteString(arg.Value.String())
		}
	}
	b.WriteString(")")
	return b.String()
}

// HaltAction instructs the runtime to halt rule execution.
type HaltAction struct{}

func (a HaltAction) Type() ActionType { return ActionHalt }

func (a HaltAction) String() string {
	return "(halt)"
}

// BindAction assigns the result of a value or computation to a local variable.
type BindAction struct {
	Variable string // e.g. "<total>" or "total"
	Value    Value  // resolved value, vector, or TypeCompute
}

func (a BindAction) Type() ActionType { return ActionBind }

func (a BindAction) String() string {
	varName := a.Variable
	if !strings.HasPrefix(varName, "<") {
		varName = "<" + varName + ">"
	}
	return fmt.Sprintf("(bind %s %s)", varName, a.Value.String())
}

// CBindAction binds the last element added to working memory (by make, modify, or call) to an element variable.
type CBindAction struct {
	Variable string // e.g. "<p>" or "p"
}

func (a CBindAction) Type() ActionType { return ActionCBind }

func (a CBindAction) String() string {
	varName := a.Variable
	if !strings.HasPrefix(varName, "<") {
		varName = "<" + varName + ">"
	}
	return fmt.Sprintf("(cbind %s)", varName)
}

// OpenFileAction opens a file and binds it to a logical name.
type OpenFileAction struct {
	LogicalName string
	Filespec    Value  // string, symbol, or variable
	Mode        string // "in", "out", "append"
}

func (a OpenFileAction) Type() ActionType { return ActionOpenFile }

func (a OpenFileAction) String() string {
	return fmt.Sprintf("(openfile %s %s %s)", a.LogicalName, a.Filespec.String(), a.Mode)
}

// CloseFileAction closes a file associated with a logical name.
type CloseFileAction struct {
	LogicalName string
}

func (a CloseFileAction) Type() ActionType { return ActionCloseFile }

func (a CloseFileAction) String() string {
	return fmt.Sprintf("(closefile %s)", a.LogicalName)
}

// DefaultAction directs the default stream for accept, write, or trace.
type DefaultAction struct {
	LogicalName string // logical name or "nil"/"terminal" to restore standard I/O
	Subsystem   string // "accept", "write", or "trace"
}

func (a DefaultAction) Type() ActionType { return ActionDefault }

func (a DefaultAction) String() string {
	return fmt.Sprintf("(default %s %s)", a.LogicalName, a.Subsystem)
}

// CustomAction executes an arbitrary user function during firing.
type CustomAction struct {
	Name    string
	Execute func(ctx any) error
}

func (a CustomAction) Type() ActionType { return ActionCustom }

func (a CustomAction) String() string {
	return fmt.Sprintf("(call %s)", a.Name)
}
