package rete

import (
	"fmt"
	"strings"

	"ops5/pkg/model"
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

// Token represents a chain of matching WMEs in the Beta network.
type Token struct {
	Parent   *Token
	WME      *model.WME
	Bindings map[string]model.Value
	Tag      PropagationTag
}

// NewToken creates a child token with an added WME and merged variable bindings.
func NewToken(parent *Token, wme *model.WME, newBindings map[string]model.Value) *Token {
	bindings := make(map[string]model.Value)
	if parent != nil {
		for k, v := range parent.Bindings {
			bindings[k] = v
		}
	}
	for k, v := range newBindings {
		bindings[k] = v
	}

	return &Token{
		Parent:   parent,
		WME:      wme,
		Bindings: bindings,
		Tag:      TagAdd,
	}
}

// DummyRootToken represents the top of the beta network.
func DummyRootToken() *Token {
	return &Token{
		Parent:   nil,
		WME:      nil,
		Bindings: make(map[string]model.Value),
		Tag:      TagAdd,
	}
}

// WMEs returns all non-nil WMEs contained in this token chain, in order from first CE to last.
func (t *Token) WMEs() []*model.WME {
	var list []*model.WME
	curr := t
	for curr != nil {
		if curr.WME != nil {
			list = append(list, curr.WME)
		}
		curr = curr.Parent
	}

	// Reverse to get chronological/CE order
	for i, j := 0, len(list)-1; i < j; i, j = i+1, j-1 {
		list[i], list[j] = list[j], list[i]
	}
	return list
}

// Timetags returns the list of timetags for all WMEs in this token.
func (t *Token) Timetags() []int64 {
	wmes := t.WMEs()
	tags := make([]int64, len(wmes))
	for i, w := range wmes {
		tags[i] = w.Timetag
	}
	return tags
}

// String returns a readable representation of the token.
func (t *Token) String() string {
	var parts []string
	for _, w := range t.WMEs() {
		parts = append(parts, fmt.Sprintf("%d", w.Timetag))
	}
	return fmt.Sprintf("Token[wmes=[%s]]", strings.Join(parts, ", "))
}
