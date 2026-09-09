package model

import (
	"fmt"
	"strings"
)

// Rule represents an OPS5 production (LHS condition elements -> RHS actions).
type Rule struct {
	Index       int                 // Declaration sequence index (used as final tie-breaker)
	Name        string              // Rule name
	Conditions  []*ConditionElement // LHS conditions
	Actions     []Action            // RHS actions
	specificity int                 // Cached specificity score
}

// NewRule creates a new Rule with the given name.
func NewRule(name string) *Rule {
	return &Rule{
		Name:        name,
		Conditions:  make([]*ConditionElement, 0),
		Actions:     make([]Action, 0),
		specificity: -1,
	}
}

// AddCondition appends a condition element to the LHS.
func (r *Rule) AddCondition(ce *ConditionElement) *Rule {
	r.Conditions = append(r.Conditions, ce)
	r.specificity = -1 // invalidate cached specificity
	return r
}

// AddAction appends an action to the RHS.
func (r *Rule) AddAction(action Action) *Rule {
	r.Actions = append(r.Actions, action)
	return r
}

// Specificity returns the computed specificity of the rule LHS.
// In OPS5:
// Specificity is the sum of tests in all condition elements:
// - Class match (+1 per CE)
// - Attribute constraints (+1 per constraint)
func (r *Rule) Specificity() int {
	if r.specificity != -1 {
		return r.specificity
	}
	score := 0
	for _, ce := range r.Conditions {
		score += ce.SpecificityScore()
	}
	r.specificity = score
	return score
}

// String returns a representation of the production.
func (r *Rule) String() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("(p %s\n", r.Name))
	for _, ce := range r.Conditions {
		b.WriteString(fmt.Sprintf("   %s\n", ce.String()))
	}
	b.WriteString("   -->\n")
	for _, act := range r.Actions {
		b.WriteString(fmt.Sprintf("   %v\n", act))
	}
	b.WriteString(")")
	return b.String()
}
