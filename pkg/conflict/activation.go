package conflict

import (
	"fmt"
	"strings"

	"ops5/pkg/model"
	"ops5/pkg/rete"
)

// Activation represents an instantiation of a rule with matched WMEs.
type Activation struct {
	Rule     *model.Rule
	Token    *rete.Token
	Timetags []int64 // Timetags in condition element order
}

// NewActivation creates a new rule activation from a terminal token.
func NewActivation(rule *model.Rule, token *rete.Token) *Activation {
	return &Activation{
		Rule:     rule,
		Token:    token,
		Timetags: token.Timetags(),
	}
}

// Key returns the unique refraction key for this instantiation (RuleName + Timetags).
func (a *Activation) Key() string {
	var tags []string
	for _, t := range a.Timetags {
		tags = append(tags, fmt.Sprintf("%d", t))
	}
	return fmt.Sprintf("%s:[%s]", a.Rule.Name, strings.Join(tags, ","))
}

// Salience returns the priority weight of the rule associated with this activation.
func (a *Activation) Salience() int {
	if a.Rule != nil {
		return a.Rule.Salience
	}
	return 0
}

// Specificity returns the specificity of the rule associated with this activation.
func (a *Activation) Specificity() int {
	return a.Rule.Specificity()
}

// String returns a human-readable representation of the activation.
func (a *Activation) String() string {
	if a.Salience() != 0 {
		return fmt.Sprintf("Activation{%s, salience=%d, timetags=%v, specificity=%d}", a.Rule.Name, a.Salience(), a.Timetags, a.Specificity())
	}
	return fmt.Sprintf("Activation{%s, timetags=%v, specificity=%d}", a.Rule.Name, a.Timetags, a.Specificity())
}
