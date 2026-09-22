package conflict

import (
	"sort"
)

// StrategyType indicates whether LEX or MEA conflict resolution is active.
type StrategyType int

const (
	StrategyLEX StrategyType = iota
	StrategyMEA
)

func (s StrategyType) String() string {
	switch s {
	case StrategyLEX:
		return "LEX"
	case StrategyMEA:
		return "MEA"
	default:
		return "UNKNOWN"
	}
}

// CompareVectors compares two slices of timetags (expected to be already sorted descending).
// Returns +1 if va dominates vb, -1 if vb dominates va, and 0 if identical.
func CompareVectors(va, vb []int64) int {
	minLen := len(va)
	if len(vb) < minLen {
		minLen = len(vb)
	}

	for i := 0; i < minLen; i++ {
		if va[i] > vb[i] {
			return 1
		} else if va[i] < vb[i] {
			return -1
		}
	}

	if len(va) > len(vb) {
		return 1
	} else if len(va) < len(vb) {
		return -1
	}

	return 0
}

func sortedDescending(tags []int64) []int64 {
	cp := make([]int64, len(tags))
	copy(cp, tags)
	sort.Slice(cp, func(i, j int) bool {
		return cp[i] > cp[j]
	})
	return cp
}

// LexCompare compares two activations using the OPS5 LEX conflict resolution strategy.
// 0. Salience: Higher salience priority dominates.
// 1. Recency: Descending sort of all timetags, compared lexicographically.
// 2. Specificity: Number of tests on the LHS.
// 3. Tie-breaker: Rule declaration sequence index.
func LexCompare(a, b *Activation) int {
	// 0. Compare salience (Tier 1)
	if a.Salience() > b.Salience() {
		return 1
	} else if a.Salience() < b.Salience() {
		return -1
	}

	// 1. Compare recency vectors
	va := a.sortedTimetags
	if len(va) == 0 && len(a.Timetags) > 0 {
		va = sortedDescending(a.Timetags)
	}
	vb := b.sortedTimetags
	if len(vb) == 0 && len(b.Timetags) > 0 {
		vb = sortedDescending(b.Timetags)
	}
	if cmp := CompareVectors(va, vb); cmp != 0 {
		return cmp
	}

	// 2. Compare specificity
	specA := a.Specificity()
	specB := b.Specificity()
	if specA > specB {
		return 1
	} else if specA < specB {
		return -1
	}

	// 3. Rule index tie-breaker (lower index = declared earlier = higher dominance)
	if a.Rule != nil && b.Rule != nil {
		if a.Rule.Index < b.Rule.Index {
			return 1
		} else if a.Rule.Index > b.Rule.Index {
			return -1
		}

		// 4. Alphabetical tie-breaker if index is equal
		if a.Rule.Name < b.Rule.Name {
			return 1
		} else if a.Rule.Name > b.Rule.Name {
			return -1
		}
	}

	return 0
}

// MeaCompare compares two activations using the OPS5 MEA conflict resolution strategy.
// 0. Salience: Higher salience priority dominates.
// 1. Recency of CE 1: Timetag of the first condition element.
// 2. Recency of remaining CEs: Descending sort of remaining timetags, compared lexicographically.
// 3. Specificity: Number of tests on the LHS.
// 4. Tie-breaker: Rule declaration sequence index.
func MeaCompare(a, b *Activation) int {
	// 0. Compare salience (Tier 1)
	if a.Salience() > b.Salience() {
		return 1
	} else if a.Salience() < b.Salience() {
		return -1
	}

	// 1. Compare timetag of condition element 1
	var firstA, firstB int64
	if len(a.Timetags) > 0 {
		firstA = a.Timetags[0]
	}
	if len(b.Timetags) > 0 {
		firstB = b.Timetags[0]
	}

	if firstA > firstB {
		return 1
	} else if firstA < firstB {
		return -1
	}

	// 2. Compare remaining timetags sorted descending
	remA := a.remainingMEA
	if len(remA) == 0 && len(a.Timetags) > 1 {
		remA = sortedDescending(a.Timetags[1:])
	}
	remB := b.remainingMEA
	if len(remB) == 0 && len(b.Timetags) > 1 {
		remB = sortedDescending(b.Timetags[1:])
	}

	if cmp := CompareVectors(remA, remB); cmp != 0 {
		return cmp
	}

	// 3. Compare specificity
	specA := a.Specificity()
	specB := b.Specificity()
	if specA > specB {
		return 1
	} else if specA < specB {
		return -1
	}

	// 4. Rule index tie-breaker
	if a.Rule.Index < b.Rule.Index {
		return 1
	} else if a.Rule.Index > b.Rule.Index {
		return -1
	}

	if a.Rule.Name < b.Rule.Name {
		return 1
	} else if a.Rule.Name > b.Rule.Name {
		return -1
	}

	return 0
}
