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

// Token represents a chain of matching WMEs in the Beta network.
type Token struct {
	Parent        *Token
	WME           *model.WME
	Bindings      map[string]model.Value
	Tag           PropagationTag
	ExtraTimetags []int64
	sig           string
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

// NewToken creates a child token with an added WME and merged variable bindings.
func NewToken(parent *Token, wme *model.WME, newBindings map[string]model.Value) *Token {
	var bindings map[string]model.Value
	if len(newBindings) == 0 {
		if parent != nil {
			bindings = parent.Bindings
		} else {
			bindings = make(map[string]model.Value)
		}
	} else if parent == nil || len(parent.Bindings) == 0 {
		bindings = newBindings
	} else {
		bindings = make(map[string]model.Value, len(parent.Bindings)+len(newBindings))
		for k, v := range parent.Bindings {
			bindings[k] = v
		}
		for k, v := range newBindings {
			bindings[k] = v
		}
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
		Bindings: bindings,
		Tag:      TagAdd,
		sig:      sig,
	}
}

// NewAccumulateToken creates a child token representing an aggregated result with extra timetags.
func NewAccumulateToken(parent *Token, newBindings map[string]model.Value, extraTimetags []int64) *Token {
	var bindings map[string]model.Value
	if len(newBindings) == 0 {
		if parent != nil {
			bindings = parent.Bindings
		} else {
			bindings = make(map[string]model.Value)
		}
	} else if parent == nil || len(parent.Bindings) == 0 {
		bindings = newBindings
	} else {
		bindings = make(map[string]model.Value, len(parent.Bindings)+len(newBindings))
		for k, v := range parent.Bindings {
			bindings[k] = v
		}
		for k, v := range newBindings {
			bindings[k] = v
		}
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
		Bindings:      bindings,
		Tag:           TagAdd,
		ExtraTimetags: extraTimetags,
		sig:           sig,
	}
}

// DummyRootToken represents the top of the beta network.
func DummyRootToken() *Token {
	return &Token{
		Parent:   nil,
		WME:      nil,
		Bindings: make(map[string]model.Value),
		Tag:      TagAdd,
		sig:      "[]",
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

// Timetags returns the list of timetags for all WMEs and extra timetags in this token in condition element order.
func (t *Token) Timetags() []int64 {
	var chain []*Token
	curr := t
	for curr != nil {
		chain = append(chain, curr)
		curr = curr.Parent
	}

	var tags []int64
	for i := len(chain) - 1; i >= 0; i-- {
		tok := chain[i]
		if tok.WME != nil {
			tags = append(tags, tok.WME.Timetag)
		}
		if len(tok.ExtraTimetags) > 0 {
			tags = append(tags, tok.ExtraTimetags...)
		}
	}
	return tags
}

// String returns a readable representation of the token.
func (t *Token) String() string {
	tags := t.Timetags()
	var parts []string
	for _, tt := range tags {
		parts = append(parts, fmt.Sprintf("%d", tt))
	}
	return fmt.Sprintf("Token[timetags=[%s]]", strings.Join(parts, ", "))
}
