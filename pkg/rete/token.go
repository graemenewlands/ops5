package rete

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/graemenewlands/ops5/pkg/model"
)

// PropagationTag indicates whether an activation is being asserted (added) or retracted (removed).
type PropagationTag int

const (
	TagAdd PropagationTag = iota
	TagRemove
)

func (t PropagationTag) String() string {
	switch t {
	case TagAdd:
		return "+ADD"
	case TagRemove:
		return "-REMOVE"
	default:
		return "UNKNOWN"
	}
}

// Binding represents an individual variable binding introduced at a condition element.
type Binding struct {
	Name  string
	Value model.Value
}

// Token represents a chain of matching WMEs along an ancestor spine in the Beta network.
// Variable bindings are maintained incrementally along the spine without copying parent bindings.
type Token struct {
	Parent        *Token
	WME           *model.WME
	Tag           PropagationTag
	ExtraTimetags []int64
	sig           string
	bindings      []Binding
}

// Signature returns the cached unique signature string for this token.
func (t *Token) Signature() string {
	if t == nil {
		return ""
	}
	if t.sig != "" {
		return t.sig
	}
	return fmt.Sprintf("%v", t.Timetags())
}

// GetBinding returns the value bound to the specified variable name, searching up the ancestor spine.
// This is an allocation-free O(depth) operation over small L1-cached slices.
func (t *Token) GetBinding(name string) (model.Value, bool) {
	for curr := t; curr != nil; curr = curr.Parent {
		for i := len(curr.bindings) - 1; i >= 0; i-- {
			if curr.bindings[i].Name == name {
				return curr.bindings[i].Value, true
			}
		}
	}
	return model.Value{}, false
}

// Bindings returns a map containing all variable bindings accumulated along this token spine.
// Materialized on demand when needed for RHS execution or external inspection.
func (t *Token) Bindings() map[string]model.Value {
	if t == nil {
		return nil
	}
	var chain []*Token
	for curr := t; curr != nil; curr = curr.Parent {
		chain = append(chain, curr)
	}
	m := make(map[string]model.Value)
	for i := len(chain) - 1; i >= 0; i-- {
		for _, b := range chain[i].bindings {
			m[b.Name] = b.Value
		}
	}
	return m
}

// LocalBindings returns the bindings directly introduced by this token node.
func (t *Token) LocalBindings() []Binding {
	if t == nil {
		return nil
	}
	return t.bindings
}

// NewToken creates a child token with an added WME and local variable bindings.
func NewToken(parent *Token, wme *model.WME, bindings []Binding) *Token {
	var b []Binding
	if len(bindings) > 0 {
		b = make([]Binding, len(bindings))
		copy(b, bindings)
	}

	var sig string
	if wme != nil {
		wmeTagStr := strconv.FormatInt(wme.Timetag, 10)
		if parent == nil || parent.sig == "" || parent.sig == "[]" {
			sig = "[" + wmeTagStr + "]"
		} else {
			sig = parent.sig[:len(parent.sig)-1] + " " + wmeTagStr + "]"
		}
	} else {
		if parent != nil {
			sig = parent.sig
		} else {
			sig = "[]"
		}
	}

	return &Token{
		Parent:   parent,
		WME:      wme,
		bindings: b,
		Tag:      TagAdd,
		sig:      sig,
	}
}

// NewTokenWithMap creates a child token using a map of variable bindings (convenience/test helper).
func NewTokenWithMap(parent *Token, wme *model.WME, newBindings map[string]model.Value) *Token {
	if len(newBindings) == 0 {
		return NewToken(parent, wme, nil)
	}
	slice := make([]Binding, 0, len(newBindings))
	for k, v := range newBindings {
		slice = append(slice, Binding{Name: k, Value: v})
	}
	return NewToken(parent, wme, slice)
}

// NewAccumulateToken creates a child token representing an aggregated result with extra timetags.
func NewAccumulateToken(parent *Token, bindings []Binding, extraTimetags []int64) *Token {
	var b []Binding
	if len(bindings) > 0 {
		b = make([]Binding, len(bindings))
		copy(b, bindings)
	}

	var sig string
	if parent != nil && parent.sig != "" && parent.sig != "[]" {
		if len(extraTimetags) > 0 {
			var sb strings.Builder
			sb.WriteString(parent.sig[:len(parent.sig)-1])
			for _, tt := range extraTimetags {
				sb.WriteString(" ")
				sb.WriteString(strconv.FormatInt(tt, 10))
			}
			sb.WriteString("]")
			sig = sb.String()
		} else {
			sig = parent.sig
		}
	} else {
		if len(extraTimetags) > 0 {
			var sb strings.Builder
			sb.WriteString("[")
			for i, tt := range extraTimetags {
				if i > 0 {
					sb.WriteString(" ")
				}
				sb.WriteString(strconv.FormatInt(tt, 10))
			}
			sb.WriteString("]")
			sig = sb.String()
		} else {
			sig = "[]"
		}
	}

	return &Token{
		Parent:        parent,
		WME:           nil,
		bindings:      b,
		Tag:           TagAdd,
		ExtraTimetags: extraTimetags,
		sig:           sig,
	}
}

// NewAccumulateTokenWithMap creates a child token representing an aggregated result with map bindings.
func NewAccumulateTokenWithMap(parent *Token, newBindings map[string]model.Value, extraTimetags []int64) *Token {
	if len(newBindings) == 0 {
		return NewAccumulateToken(parent, nil, extraTimetags)
	}
	slice := make([]Binding, 0, len(newBindings))
	for k, v := range newBindings {
		slice = append(slice, Binding{Name: k, Value: v})
	}
	return NewAccumulateToken(parent, slice, extraTimetags)
}

// DummyRootToken represents the top of the beta network.
func DummyRootToken() *Token {
	return &Token{
		Parent:   nil,
		WME:      nil,
		bindings: nil,
		Tag:      TagAdd,
		sig:      "[]",
	}
}

// WMEs returns all non-nil WMEs contained in this token chain, in order from first CE to last.
func (t *Token) WMEs() []*model.WME {
	if t == nil {
		return nil
	}
	count := 0
	for curr := t; curr != nil; curr = curr.Parent {
		if curr.WME != nil {
			count++
		}
	}
	if count == 0 {
		return nil
	}
	list := make([]*model.WME, count)
	idx := count - 1
	for curr := t; curr != nil; curr = curr.Parent {
		if curr.WME != nil {
			list[idx] = curr.WME
			idx--
		}
	}
	return list
}

// Timetags returns the list of timetags for all WMEs and extra timetags in this token in condition element order.
func (t *Token) Timetags() []int64 {
	if t == nil {
		return nil
	}
	count := 0
	for curr := t; curr != nil; curr = curr.Parent {
		if curr.WME != nil {
			count++
		}
		count += len(curr.ExtraTimetags)
	}
	if count == 0 {
		return nil
	}

	tags := make([]int64, count)
	idx := count - 1
	for curr := t; curr != nil; curr = curr.Parent {
		for i := len(curr.ExtraTimetags) - 1; i >= 0; i-- {
			tags[idx] = curr.ExtraTimetags[i]
			idx--
		}
		if curr.WME != nil {
			tags[idx] = curr.WME.Timetag
			idx--
		}
	}
	return tags
}

// String returns a readable representation of the token.
func (t *Token) String() string {
	tags := t.Timetags()
	var sb strings.Builder
	sb.WriteString("Token[timetags=[")
	for i, tt := range tags {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(strconv.FormatInt(tt, 10))
	}
	sb.WriteString("]]")
	return sb.String()
}
