package conflict

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/graemenewlands/ops5/pkg/model"
	"github.com/graemenewlands/ops5/pkg/rete"
)

// Activation represents an instantiation of a rule with matched WMEs.
type Activation struct {
	Rule           *model.Rule
	Token          *rete.Token
	Timetags       []int64 // Timetags in condition element order
	sortedTimetags []int64 // Pre-sorted descending for LEX
	remainingMEA   []int64 // Pre-sorted descending (excluding CE 1) for MEA
	key            string  // Cached refraction key
	heapIndex      int     // Current index in agenda binary heap (-1 if not in heap)
}

// NewActivation creates a new rule activation from a terminal token.
func NewActivation(rule *model.Rule, token *rete.Token) *Activation {
	timetags := token.Timetags()

	// Pre-sort timetags descending for LEX
	sortedTags := make([]int64, len(timetags))
	copy(sortedTags, timetags)
	sort.Slice(sortedTags, func(i, j int) bool {
		return sortedTags[i] > sortedTags[j]
	})

	// Pre-sort remaining timetags descending for MEA
	var remMEA []int64
	if len(timetags) > 1 {
		remMEA = make([]int64, len(timetags)-1)
		copy(remMEA, timetags[1:])
		sort.Slice(remMEA, func(i, j int) bool {
			return remMEA[i] > remMEA[j]
		})
	}

	var ruleName string
	if rule != nil {
		ruleName = rule.Name
	}
	var sb strings.Builder
	sb.WriteString(ruleName)
	sb.WriteString(":[")
	for i, t := range timetags {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(strconv.FormatInt(t, 10))
	}
	sb.WriteString("]")

	return &Activation{
		Rule:           rule,
		Token:          token,
		Timetags:       timetags,
		sortedTimetags: sortedTags,
		remainingMEA:   remMEA,
		key:            sb.String(),
		heapIndex:      -1,
	}
}

// Key returns the unique refraction key for this instantiation (RuleName + Timetags).
func (a *Activation) Key() string {
	if a == nil {
		return ""
	}
	if a.key != "" {
		return a.key
	}
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
