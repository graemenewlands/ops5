package rete

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/graemenewlands/ops5/pkg/model"
)

type terminalInfo struct {
	terminal *TerminalNode
	parent   BetaNode
}

// betaNodeEntry tracks a shared beta node and its downstream BetaMemory.
type betaNodeEntry struct {
	key       string
	parentMem *BetaMemory
	node      BetaNode
	alphaMem  *AlphaMemory
	betaMem   *BetaMemory
	rules     map[string]bool
}

// RuleNodeInfo tracks the compiled Rete nodes associated with a rule for diagnostic inspection (e.g. matches).
type RuleNodeInfo struct {
	Rule      *model.Rule
	AlphaMems []*AlphaMemory // 1 per condition element (nil if IsTest or IsNCC)
	BetaMems  []*BetaMemory  // 1 after each condition element except the last
	Terminal  *TerminalNode
}

// Network coordinates the Alpha and Beta networks and compiles rules into Rete nodes.
type Network struct {
	mu            sync.RWMutex
	alphaRoot     *AlphaRootNode
	rootBetaMem   *BetaMemory
	alphaMemPool  map[string]*AlphaMemory
	betaNodePool  map[string]*betaNodeEntry
	ruleBetaNodes map[string][]*betaNodeEntry
	nextBetaMemID        int
	terminals            map[string]terminalInfo
	ruleNodeInfos        map[string]*RuleNodeInfo
	joinOptimizerEnabled bool
}

// NewNetwork creates an initialized Rete network.
func NewNetwork() *Network {
	net := &Network{
		alphaRoot:            NewAlphaRootNode(),
		rootBetaMem:          NewBetaMemory(),
		alphaMemPool:         make(map[string]*AlphaMemory),
		betaNodePool:         make(map[string]*betaNodeEntry),
		ruleBetaNodes:        make(map[string][]*betaNodeEntry),
		nextBetaMemID:        1,
		terminals:            make(map[string]terminalInfo),
		ruleNodeInfos:        make(map[string]*RuleNodeInfo),
		joinOptimizerEnabled: true,
	}
	net.rootBetaMem.id = 0
	// Seed the root BetaMemory with the dummy token
	net.rootBetaMem.LeftActivation(DummyRootToken(), TagAdd)
	return net
}

// SetJoinOptimizer enables or disables the static join ordering heuristic optimizer.
func (net *Network) SetJoinOptimizer(enabled bool) {
	net.mu.Lock()
	defer net.mu.Unlock()
	net.joinOptimizerEnabled = enabled
}

// JoinOptimizerEnabled returns whether the static join ordering optimizer is enabled.
func (net *Network) JoinOptimizerEnabled() bool {
	net.mu.RLock()
	defer net.mu.RUnlock()
	return net.joinOptimizerEnabled
}


// OnAssert implements wm.Listener to route WME assertions into the alpha network.
func (net *Network) OnAssert(wme *model.WME) {
	net.alphaRoot.Activation(wme, TagAdd)
}

// OnRetract implements wm.Listener to route WME retractions into the alpha network.
func (net *Network) OnRetract(wme *model.WME) {
	net.alphaRoot.Activation(wme, TagRemove)
}

type alphaTestSpec struct {
	Attribute   string
	VectorIndex int
	IsEquality  bool
	Op          model.Operator
	Value       model.Value
	Disjunction []model.TestConstraint
}

func extractConstantTests(ce *model.ConditionElement) (eqTests []alphaTestSpec, nonEqTests []alphaTestSpec) {
	for _, at := range ce.Tests {
		isMulti := len(at.Constraints) > 1
		for idx, c := range at.Constraints {
			vecIdx := -1
			if isMulti {
				vecIdx = idx
			}
			if len(c.Disjunction) > 0 {
				hasVar := false
				for _, dj := range c.Disjunction {
					if dj.Value.IsVariable() {
						hasVar = true
						break
					}
				}
				if !hasVar {
					nonEqTests = append(nonEqTests, alphaTestSpec{
						Attribute:   model.NormalizeAttribute(at.Attribute),
						VectorIndex: vecIdx,
						IsEquality:  false,
						Disjunction: c.Disjunction,
					})
				}
			} else if !c.Value.IsVariable() {
				normAttr := model.NormalizeAttribute(at.Attribute)
				if c.Op == model.OpEqual {
					eqTests = append(eqTests, alphaTestSpec{
						Attribute:   normAttr,
						VectorIndex: vecIdx,
						IsEquality:  true,
						Op:          c.Op,
						Value:       c.Value,
					})
				} else {
					nonEqTests = append(nonEqTests, alphaTestSpec{
						Attribute:   normAttr,
						VectorIndex: vecIdx,
						IsEquality:  false,
						Op:          c.Op,
						Value:       c.Value,
					})
				}
			}
		}
	}

	// Sort eqTests canonically: Attribute asc, VectorIndex asc, Value string asc
	sort.Slice(eqTests, func(i, j int) bool {
		if eqTests[i].Attribute != eqTests[j].Attribute {
			return eqTests[i].Attribute < eqTests[j].Attribute
		}
		if eqTests[i].VectorIndex != eqTests[j].VectorIndex {
			return eqTests[i].VectorIndex < eqTests[j].VectorIndex
		}
		return eqTests[i].Value.String() < eqTests[j].Value.String()
	})

	// Sort nonEqTests canonically: Attribute asc, VectorIndex asc, Op asc, Value string asc
	sort.Slice(nonEqTests, func(i, j int) bool {
		if nonEqTests[i].Attribute != nonEqTests[j].Attribute {
			return nonEqTests[i].Attribute < nonEqTests[j].Attribute
		}
		if nonEqTests[i].VectorIndex != nonEqTests[j].VectorIndex {
			return nonEqTests[i].VectorIndex < nonEqTests[j].VectorIndex
		}
		if nonEqTests[i].Op != nonEqTests[j].Op {
			return nonEqTests[i].Op < nonEqTests[j].Op
		}
		return nonEqTests[i].Value.String() < nonEqTests[j].Value.String()
	})

	return eqTests, nonEqTests
}

// getAlphaKey produces a canonical key for sharing AlphaMemory across equivalent conditions.
func getAlphaKey(ce *model.ConditionElement) string {
	eqTests, nonEqTests := extractConstantTests(ce)
	var sb strings.Builder
	sb.WriteString(ce.Class)
	for _, eq := range eqTests {
		if eq.VectorIndex >= 0 {
			sb.WriteString(fmt.Sprintf("|%s[%d]=%s", eq.Attribute, eq.VectorIndex, eq.Value.String()))
		} else {
			sb.WriteString(fmt.Sprintf("|%s=%s", eq.Attribute, eq.Value.String()))
		}
	}
	for _, ne := range nonEqTests {
		if len(ne.Disjunction) > 0 {
			var parts []string
			for _, dj := range ne.Disjunction {
				parts = append(parts, fmt.Sprintf("%s%s", dj.Op.String(), dj.Value.String()))
			}
			if ne.VectorIndex >= 0 {
				sb.WriteString(fmt.Sprintf("|%s[%d]<<%s>>", ne.Attribute, ne.VectorIndex, strings.Join(parts, ",")))
			} else {
				sb.WriteString(fmt.Sprintf("|%s<<%s>>", ne.Attribute, strings.Join(parts, ",")))
			}
		} else {
			if ne.VectorIndex >= 0 {
				sb.WriteString(fmt.Sprintf("|%s[%d]%s%s", ne.Attribute, ne.VectorIndex, ne.Op.String(), ne.Value.String()))
			} else {
				sb.WriteString(fmt.Sprintf("|%s%s%s", ne.Attribute, ne.Op.String(), ne.Value.String()))
			}
		}
	}
	return sb.String()
}

func canonicalCEString(ce *model.ConditionElement) string {
	if ce == nil {
		return ""
	}
	if ce.IsTest {
		return ce.String()
	}
	if ce.IsNCC {
		var parts []string
		for _, sub := range ce.NCCConditions {
			parts = append(parts, canonicalCEString(sub))
		}
		return "-(" + strings.Join(parts, " ") + ")"
	}
	var sb strings.Builder
	if ce.ElementVariable != "" {
		sb.WriteString(fmt.Sprintf("<%s> ", ce.ElementVariable))
	}
	if ce.IsNegative {
		sb.WriteString("-(")
	} else if ce.IsExistential {
		sb.WriteString("(exists (")
	} else if ce.IsAccumulate && ce.Accumulate != nil {
		sb.WriteString("(accumulate (")
	} else {
		sb.WriteString("(")
	}
	sb.WriteString(ce.Class)

	// Collect and sort attribute tests by attribute name for canonical matching
	type attrEntry struct {
		name string
		repr string
	}
	var entries []attrEntry
	for _, at := range ce.Tests {
		var b strings.Builder
		b.WriteString(fmt.Sprintf("^%s", at.Attribute))
		for _, c := range at.Constraints {
			b.WriteString(" ")
			b.WriteString(c.String())
		}
		entries = append(entries, attrEntry{name: at.Attribute, repr: b.String()})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].name != entries[j].name {
			return entries[i].name < entries[j].name
		}
		return entries[i].repr < entries[j].repr
	})
	for _, e := range entries {
		sb.WriteString(" ")
		sb.WriteString(e.repr)
	}

	if ce.IsExistential {
		sb.WriteString("))")
	} else if ce.IsAccumulate && ce.Accumulate != nil {
		sb.WriteString(fmt.Sprintf(") %s", ce.Accumulate.Op.String()))
		if ce.Accumulate.Target.String() != "" && ce.Accumulate.Target.String() != "nil" && ce.Accumulate.Target.String() != `""` {
			sb.WriteString(" ")
			sb.WriteString(ce.Accumulate.Target.String())
		}
		sb.WriteString(fmt.Sprintf(" <%s>)", ce.Accumulate.ResultVar))
	} else {
		sb.WriteString(")")
	}
	return sb.String()
}

func joinTestsKey(tests []JoinTest) string {
	if len(tests) == 0 {
		return ""
	}
	var parts []string
	for _, jt := range tests {
		if len(jt.Disjunction) > 0 {
			var djs []string
			for _, d := range jt.Disjunction {
				djs = append(djs, fmt.Sprintf("%s%s", d.Op.String(), d.Value.String()))
			}
			parts = append(parts, fmt.Sprintf("%s[%d]<<%s>>", jt.Attribute, jt.VectorIndex, strings.Join(djs, ",")))
		} else {
			parts = append(parts, fmt.Sprintf("%s[%d]%s<%s>", jt.Attribute, jt.VectorIndex, jt.Op.String(), jt.Variable))
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, ";")
}

func betaStepKey(parentID int, ce *model.ConditionElement, joinTests []JoinTest) string {
	return fmt.Sprintf("bm%d|%s|jt:%s", parentID, canonicalCEString(ce), joinTestsKey(joinTests))
}

func (net *Network) newBetaMemoryLocked() *BetaMemory {
	bm := NewBetaMemory()
	bm.id = net.nextBetaMemID
	net.nextBetaMemID++
	return bm
}

func matchesCEConstants(ce *model.ConditionElement, wme *model.WME) bool {
	if ce.Class != "*" && ce.Class != wme.Class {
		return false
	}
	for _, at := range ce.Tests {
		isMulti := len(at.Constraints) > 1
		for idx, c := range at.Constraints {
			vecIdx := -1
			if isMulti {
				vecIdx = idx
			}
			if len(c.Disjunction) > 0 {
				hasVar := false
				for _, dj := range c.Disjunction {
					if dj.Value.IsVariable() {
						hasVar = true
						break
					}
				}
				if !hasVar {
					ct := NewIndexedDisjunctiveConstantTestNode(at.Attribute, c.Disjunction, vecIdx)
					if !ct.Test(wme) {
						return false
					}
				}
			} else if !c.Value.IsVariable() {
				ct := NewIndexedConstantTestNode(at.Attribute, c.Op, c.Value, vecIdx)
				if !ct.Test(wme) {
					return false
				}
			}
		}
	}
	return true
}

type alphaBuilderCursor interface {
	getOrCreateSwitchNode(attr string, vecIdx int) *AlphaSwitchNode
	getOrCreateConstantTestNode(attr string, op model.Operator, val model.Value, vecIdx int) *ConstantTestNode
	getOrCreateDisjunctiveTestNode(attr string, disj []model.TestConstraint, vecIdx int) *ConstantTestNode
	addSuccessor(succ AlphaNode)
}

type typeNodeCursor struct {
	tn *TypeNode
}

func (c *typeNodeCursor) getOrCreateSwitchNode(attr string, vecIdx int) *AlphaSwitchNode {
	return c.tn.GetOrCreateSwitchNode(attr, vecIdx)
}

func (c *typeNodeCursor) getOrCreateConstantTestNode(attr string, op model.Operator, val model.Value, vecIdx int) *ConstantTestNode {
	return c.tn.GetOrCreateConstantTestNode(attr, op, val, vecIdx)
}

func (c *typeNodeCursor) getOrCreateDisjunctiveTestNode(attr string, disj []model.TestConstraint, vecIdx int) *ConstantTestNode {
	return c.tn.GetOrCreateDisjunctiveTestNode(attr, disj, vecIdx)
}

func (c *typeNodeCursor) addSuccessor(succ AlphaNode) {
	c.tn.AddSuccessor(succ)
}

type constantTestCursor struct {
	ct *ConstantTestNode
}

func (c *constantTestCursor) getOrCreateSwitchNode(attr string, vecIdx int) *AlphaSwitchNode {
	return c.ct.GetOrCreateSwitchNode(attr, vecIdx)
}

func (c *constantTestCursor) getOrCreateConstantTestNode(attr string, op model.Operator, val model.Value, vecIdx int) *ConstantTestNode {
	return c.ct.GetOrCreateConstantTestNode(attr, op, val, vecIdx)
}

func (c *constantTestCursor) getOrCreateDisjunctiveTestNode(attr string, disj []model.TestConstraint, vecIdx int) *ConstantTestNode {
	return c.ct.GetOrCreateDisjunctiveTestNode(attr, disj, vecIdx)
}

func (c *constantTestCursor) addSuccessor(succ AlphaNode) {
	c.ct.AddSuccessor(succ)
}

type switchBranchCursor struct {
	sw  *AlphaSwitchNode
	key string
}

func (c *switchBranchCursor) getOrCreateSwitchNode(attr string, vecIdx int) *AlphaSwitchNode {
	return c.sw.GetOrCreateSwitchNode(c.key, attr, vecIdx)
}

func (c *switchBranchCursor) getOrCreateConstantTestNode(attr string, op model.Operator, val model.Value, vecIdx int) *ConstantTestNode {
	return c.sw.GetOrCreateConstantTestNode(c.key, attr, op, val, vecIdx)
}

func (c *switchBranchCursor) getOrCreateDisjunctiveTestNode(attr string, disj []model.TestConstraint, vecIdx int) *ConstantTestNode {
	return c.sw.GetOrCreateDisjunctiveTestNode(c.key, attr, disj, vecIdx)
}

func (c *switchBranchCursor) addSuccessor(succ AlphaNode) {
	c.sw.AddSuccessor(c.key, succ)
}

// buildAlphaMemory creates or retrieves a shared AlphaMemory for the given condition element.
func (net *Network) buildAlphaMemory(ce *model.ConditionElement, existingWMEs []*model.WME) *AlphaMemory {
	key := getAlphaKey(ce)
	if am, exists := net.alphaMemPool[key]; exists {
		return am
	}

	eqTests, nonEqTests := extractConstantTests(ce)

	var cursor alphaBuilderCursor = &typeNodeCursor{tn: net.alphaRoot.GetOrCreateTypeNode(ce.Class)}

	// 1. Process equality tests (via AlphaSwitchNodes for O(1) attribute dispatch)
	for _, eq := range eqTests {
		sw := cursor.getOrCreateSwitchNode(eq.Attribute, eq.VectorIndex)
		branchKey := CanonicalValueKey(eq.Value)
		cursor = &switchBranchCursor{sw: sw, key: branchKey}
	}

	// 2. Process non-equality tests (via ConstantTestNodes)
	for _, nonEq := range nonEqTests {
		if len(nonEq.Disjunction) > 0 {
			ct := cursor.getOrCreateDisjunctiveTestNode(nonEq.Attribute, nonEq.Disjunction, nonEq.VectorIndex)
			cursor = &constantTestCursor{ct: ct}
		} else {
			ct := cursor.getOrCreateConstantTestNode(nonEq.Attribute, nonEq.Op, nonEq.Value, nonEq.VectorIndex)
			cursor = &constantTestCursor{ct: ct}
		}
	}

	am := NewAlphaMemory()
	// Pre-populate with matching existing WMEs
	for _, wme := range existingWMEs {
		if matchesCEConstants(ce, wme) {
			am.items[wme.Timetag] = wme
		}
	}

	cursor.addSuccessor(am)
	net.alphaMemPool[key] = am
	return am
}

// AddRule compiles a rule into the Rete network and connects its terminal node to the listener.
func (net *Network) AddRule(rule *model.Rule, listener ConflictSetListener) {
	net.AddRuleWithWMEs(rule, listener, nil)
}

// AddRuleWithWMEs compiles a rule and evaluates it against existing working memory elements.
func (net *Network) AddRuleWithWMEs(rule *model.Rule, listener ConflictSetListener, existingWMEs []*model.WME) {
	net.mu.Lock()
	defer net.mu.Unlock()

	if len(rule.Conditions) == 0 {
		return
	}

	// If rule already exists in Rete network, remove previous version cleanly
	if _, exists := net.terminals[rule.Name]; exists {
		net.removeRuleLocked(rule.Name)
	}

	effectiveRule := rule
	if net.joinOptimizerEnabled && !rule.NoReorder {
		effectiveRule = OptimizeRuleJoinOrder(rule)
	}

	boundVariables := make(map[string]bool)
	currBetaMem := net.rootBetaMem
	ruleNodeInfo := &RuleNodeInfo{
		Rule: effectiveRule,
	}

	for i, ce := range effectiveRule.Conditions {
		isLast := (i == len(effectiveRule.Conditions) - 1)

		if ce.IsTest {
			ruleNodeInfo.AlphaMems = append(ruleNodeInfo.AlphaMems, nil)
			stepKey := betaStepKey(currBetaMem.id, ce, nil)

			var nextBetaMem *BetaMemory
			if entry, ok := net.betaNodePool[stepKey]; ok {
				entry.rules[rule.Name] = true
				net.ruleBetaNodes[rule.Name] = append(net.ruleBetaNodes[rule.Name], entry)
				if isLast {
					terminal := NewTerminalNode(effectiveRule, listener)
					ruleNodeInfo.Terminal = terminal
					entry.node.AddSuccessor(terminal)
					net.terminals[rule.Name] = terminalInfo{
						terminal: terminal,
						parent:   entry.node,
					}
				} else {
					if entry.betaMem == nil {
						entry.betaMem = net.newBetaMemoryLocked()
						entry.node.AddSuccessor(entry.betaMem)
					}
					nextBetaMem = entry.betaMem
					ruleNodeInfo.BetaMems = append(ruleNodeInfo.BetaMems, nextBetaMem)
					currBetaMem = nextBetaMem
				}
			} else {
				evalNode := NewEvalNode(ce.EvalTest)
				if isLast {
					terminal := NewTerminalNode(effectiveRule, listener)
					ruleNodeInfo.Terminal = terminal
					evalNode.AddSuccessor(terminal)
					evalNode.Attach(currBetaMem)

					entry := &betaNodeEntry{
						key:       stepKey,
						parentMem: currBetaMem,
						node:      evalNode,
						alphaMem:  nil,
						betaMem:   nil,
						rules:     map[string]bool{rule.Name: true},
					}
					net.betaNodePool[stepKey] = entry
					net.ruleBetaNodes[rule.Name] = append(net.ruleBetaNodes[rule.Name], entry)
					net.terminals[rule.Name] = terminalInfo{
						terminal: terminal,
						parent:   evalNode,
					}
				} else {
					nextBetaMem = net.newBetaMemoryLocked()
					evalNode.AddSuccessor(nextBetaMem)
					evalNode.Attach(currBetaMem)

					entry := &betaNodeEntry{
						key:       stepKey,
						parentMem: currBetaMem,
						node:      evalNode,
						alphaMem:  nil,
						betaMem:   nextBetaMem,
						rules:     map[string]bool{rule.Name: true},
					}
					net.betaNodePool[stepKey] = entry
					net.ruleBetaNodes[rule.Name] = append(net.ruleBetaNodes[rule.Name], entry)
					ruleNodeInfo.BetaMems = append(ruleNodeInfo.BetaMems, nextBetaMem)
					currBetaMem = nextBetaMem
				}
			}
			continue
		}

		if ce.IsNCC {
			ruleNodeInfo.AlphaMems = append(ruleNodeInfo.AlphaMems, nil)
			stepKey := betaStepKey(currBetaMem.id, ce, nil)

			var nextBetaMem *BetaMemory
			if entry, ok := net.betaNodePool[stepKey]; ok {
				entry.rules[rule.Name] = true
				net.ruleBetaNodes[rule.Name] = append(net.ruleBetaNodes[rule.Name], entry)
				if isLast {
					terminal := NewTerminalNode(effectiveRule, listener)
					ruleNodeInfo.Terminal = terminal
					entry.node.AddSuccessor(terminal)
					net.terminals[rule.Name] = terminalInfo{
						terminal: terminal,
						parent:   entry.node,
					}
				} else {
					if entry.betaMem == nil {
						entry.betaMem = net.newBetaMemoryLocked()
						entry.node.AddSuccessor(entry.betaMem)
					}
					nextBetaMem = entry.betaMem
					ruleNodeInfo.BetaMems = append(ruleNodeInfo.BetaMems, nextBetaMem)
					currBetaMem = nextBetaMem
				}
			} else {
				subConditions := ce.NCCConditions
				partner := NewNccPartnerNode(len(subConditions))
				nccNode := NewNccNode(currBetaMem, partner, ce)

				subBetaMem := currBetaMem
				subBoundVars := make(map[string]bool)
				for k, v := range boundVariables {
					subBoundVars[k] = v
				}

				for subIdx, subCE := range subConditions {
					subIsLast := (subIdx == len(subConditions) - 1)

					var subNextNode LeftActivatable
					if subIsLast {
						subNextNode = partner
					} else {
						subNextBetaMem := net.newBetaMemoryLocked()
						subNextNode = subNextBetaMem
					}

					if subCE.IsTest {
						evalNode := NewEvalNode(subCE.EvalTest)
						evalNode.AddSuccessor(subNextNode)
						subBetaMem.AddSuccessor(evalNode)
						if !subIsLast {
							subBetaMem = subNextNode.(*BetaMemory)
						}
						continue
					}

					subAlphaMem := net.buildAlphaMemory(subCE, existingWMEs)

					var subJoinTests []JoinTest
					for _, at := range subCE.Tests {
						isMulti := len(at.Constraints) > 1
						for idx, c := range at.Constraints {
							vecIdx := -1
							if isMulti {
								vecIdx = idx
							}
							if len(c.Disjunction) > 0 {
								hasBoundVar := false
								for _, dj := range c.Disjunction {
									if dj.Value.IsVariable() && subBoundVars[dj.Value.VariableName()] {
										hasBoundVar = true
										break
									}
								}
								if hasBoundVar {
									subJoinTests = append(subJoinTests, JoinTest{
										Attribute:   at.Attribute,
										VectorIndex: vecIdx,
										Disjunction: c.Disjunction,
									})
								}
							} else if c.Value.IsVariable() {
								varName := c.Value.VariableName()
								if subBoundVars[varName] {
									subJoinTests = append(subJoinTests, JoinTest{
										Attribute:   at.Attribute,
										Op:          c.Op,
										Variable:    varName,
										VectorIndex: vecIdx,
									})
								}
							}
						}
					}

					if subCE.IsNegative {
						negNode := NewNegativeJoinNode(subBetaMem, subAlphaMem, subCE, subJoinTests)
						negNode.AddSuccessor(subNextNode)
						negNode.Attach()
					} else if subCE.IsExistential {
						existNode := NewExistentialJoinNode(subBetaMem, subAlphaMem, subCE, subJoinTests)
						existNode.AddSuccessor(subNextNode)
						existNode.Attach()
					} else if subCE.IsAccumulate {
						accNode := NewAccumulateNode(subBetaMem, subAlphaMem, subCE, subCE.Accumulate, subJoinTests)
						accNode.AddSuccessor(subNextNode)
						accNode.Attach()
					} else {
						joinNode := NewJoinNode(subBetaMem, subAlphaMem, subCE, subJoinTests)
						joinNode.AddSuccessor(subNextNode)
						joinNode.Attach()
					}

					if !subCE.IsNegative && !subCE.IsExistential {
						for _, v := range subCE.Variables() {
							subBoundVars[v] = true
						}
					}

					if !subIsLast {
						subBetaMem = subNextNode.(*BetaMemory)
					}
				}

				if isLast {
					terminal := NewTerminalNode(effectiveRule, listener)
					ruleNodeInfo.Terminal = terminal
					nccNode.AddSuccessor(terminal)
					currBetaMem.AddSuccessor(nccNode)

					entry := &betaNodeEntry{
						key:       stepKey,
						parentMem: currBetaMem,
						node:      nccNode,
						alphaMem:  nil,
						betaMem:   nil,
						rules:     map[string]bool{rule.Name: true},
					}
					net.betaNodePool[stepKey] = entry
					net.ruleBetaNodes[rule.Name] = append(net.ruleBetaNodes[rule.Name], entry)
					net.terminals[rule.Name] = terminalInfo{
						terminal: terminal,
						parent:   nccNode,
					}
				} else {
					nextBetaMem = net.newBetaMemoryLocked()
					nccNode.AddSuccessor(nextBetaMem)
					currBetaMem.AddSuccessor(nccNode)

					entry := &betaNodeEntry{
						key:       stepKey,
						parentMem: currBetaMem,
						node:      nccNode,
						alphaMem:  nil,
						betaMem:   nextBetaMem,
						rules:     map[string]bool{rule.Name: true},
					}
					net.betaNodePool[stepKey] = entry
					net.ruleBetaNodes[rule.Name] = append(net.ruleBetaNodes[rule.Name], entry)
					ruleNodeInfo.BetaMems = append(ruleNodeInfo.BetaMems, nextBetaMem)
					currBetaMem = nextBetaMem
				}
			}
			continue
		}

		alphaMem := net.buildAlphaMemory(ce, existingWMEs)
		ruleNodeInfo.AlphaMems = append(ruleNodeInfo.AlphaMems, alphaMem)

		// Determine join tests: compare right WME attributes against variables already bound in previous CEs
		var joinTests []JoinTest
		for _, at := range ce.Tests {
			isMulti := len(at.Constraints) > 1
			for idx, c := range at.Constraints {
				vecIdx := -1
				if isMulti {
					vecIdx = idx
				}
				if len(c.Disjunction) > 0 {
					hasBoundVar := false
					for _, dj := range c.Disjunction {
						if dj.Value.IsVariable() && boundVariables[dj.Value.VariableName()] {
							hasBoundVar = true
							break
						}
					}
					if hasBoundVar {
						joinTests = append(joinTests, JoinTest{
							Attribute:   at.Attribute,
							VectorIndex: vecIdx,
							Disjunction: c.Disjunction,
						})
					}
				} else if c.Value.IsVariable() {
					varName := c.Value.VariableName()
					if boundVariables[varName] {
						joinTests = append(joinTests, JoinTest{
							Attribute:   at.Attribute,
							Op:          c.Op,
							Variable:    varName,
							VectorIndex: vecIdx,
						})
					}
				}
			}
		}

		stepKey := betaStepKey(currBetaMem.id, ce, joinTests)
		var nextBetaMem *BetaMemory

		if entry, ok := net.betaNodePool[stepKey]; ok {
			entry.rules[rule.Name] = true
			net.ruleBetaNodes[rule.Name] = append(net.ruleBetaNodes[rule.Name], entry)
			if isLast {
				terminal := NewTerminalNode(effectiveRule, listener)
				ruleNodeInfo.Terminal = terminal
				entry.node.AddSuccessor(terminal)
				net.terminals[rule.Name] = terminalInfo{
					terminal: terminal,
					parent:   entry.node,
				}
			} else {
				if entry.betaMem == nil {
					entry.betaMem = net.newBetaMemoryLocked()
					entry.node.AddSuccessor(entry.betaMem)
				}
				nextBetaMem = entry.betaMem
				ruleNodeInfo.BetaMems = append(ruleNodeInfo.BetaMems, nextBetaMem)
				currBetaMem = nextBetaMem
			}
		} else {
			var joinNode BetaNode
			var terminal *TerminalNode
			var downstream LeftActivatable

			if isLast {
				terminal = NewTerminalNode(effectiveRule, listener)
				ruleNodeInfo.Terminal = terminal
				downstream = terminal
			} else {
				nextBetaMem = net.newBetaMemoryLocked()
				downstream = nextBetaMem
			}

			if ce.IsNegative {
				njn := NewNegativeJoinNode(currBetaMem, alphaMem, ce, joinTests)
				njn.AddSuccessor(downstream)
				njn.Attach()
				joinNode = njn
			} else if ce.IsExistential {
				ejn := NewExistentialJoinNode(currBetaMem, alphaMem, ce, joinTests)
				ejn.AddSuccessor(downstream)
				ejn.Attach()
				joinNode = ejn
			} else if ce.IsAccumulate {
				an := NewAccumulateNode(currBetaMem, alphaMem, ce, ce.Accumulate, joinTests)
				an.AddSuccessor(downstream)
				an.Attach()
				joinNode = an
			} else {
				jn := NewJoinNode(currBetaMem, alphaMem, ce, joinTests)
				jn.AddSuccessor(downstream)
				jn.Attach()
				joinNode = jn
			}

			entry := &betaNodeEntry{
				key:       stepKey,
				parentMem: currBetaMem,
				node:      joinNode,
				alphaMem:  alphaMem,
				betaMem:   nextBetaMem,
				rules:     map[string]bool{rule.Name: true},
			}
			net.betaNodePool[stepKey] = entry
			net.ruleBetaNodes[rule.Name] = append(net.ruleBetaNodes[rule.Name], entry)

			if isLast {
				net.terminals[rule.Name] = terminalInfo{
					terminal: terminal,
					parent:   joinNode,
				}
			} else {
				ruleNodeInfo.BetaMems = append(ruleNodeInfo.BetaMems, nextBetaMem)
				currBetaMem = nextBetaMem
			}
		}

		// Update bound variables for subsequent condition elements
		if ce.IsAccumulate {
			if ce.Accumulate != nil && ce.Accumulate.ResultVar != "" {
				boundVariables[ce.Accumulate.ResultVar] = true
			}
		} else if !ce.IsNegative && !ce.IsExistential {
			for _, v := range ce.Variables() {
				boundVariables[v] = true
			}
		}
	}
	net.ruleNodeInfos[rule.Name] = ruleNodeInfo
}

// RemoveRule detaches and deactivates the terminal node for the specified rule,
// and prunes unreferenced shared beta nodes.
// Returns true if the rule was registered in the network and removed, false otherwise.
func (net *Network) RemoveRule(ruleName string) bool {
	net.mu.Lock()
	defer net.mu.Unlock()
	return net.removeRuleLocked(ruleName)
}

func (net *Network) removeRuleLocked(ruleName string) bool {
	info, ok := net.terminals[ruleName]
	if !ok {
		return false
	}

	info.terminal.Deactivate()
	if info.parent != nil {
		info.parent.RemoveSuccessor(info.terminal)
	}
	delete(net.terminals, ruleName)
	delete(net.ruleNodeInfos, ruleName)

	// Clean up shared beta nodes used by this rule (in reverse order from bottom up)
	entries := net.ruleBetaNodes[ruleName]
	delete(net.ruleBetaNodes, ruleName)

	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		delete(entry.rules, ruleName)
		if len(entry.rules) == 0 {
			if entry.parentMem != nil && entry.node != nil {
				entry.parentMem.RemoveSuccessor(entry.node)
			}
			if entry.alphaMem != nil && entry.node != nil {
				if ra, ok := entry.node.(RightActivatable); ok {
					entry.alphaMem.RemoveSuccessor(ra)
				}
			}
			if entry.betaMem != nil && entry.node != nil {
				entry.node.RemoveSuccessor(entry.betaMem)
			}
			delete(net.betaNodePool, entry.key)
		} else {
			if entry.betaMem != nil && entry.node != nil {
				stillNeeded := false
				for rName := range entry.rules {
					rEntries := net.ruleBetaNodes[rName]
					if len(rEntries) > 0 && rEntries[len(rEntries)-1] != entry {
						stillNeeded = true
						break
					}
				}
				if !stillNeeded {
					entry.node.RemoveSuccessor(entry.betaMem)
					entry.betaMem = nil
				}
			}
		}
	}
	return true
}

// BetaNodeCount returns the total number of shared beta nodes currently compiled in the Rete network.
func (net *Network) BetaNodeCount() int {
	net.mu.RLock()
	defer net.mu.RUnlock()
	return len(net.betaNodePool)
}

// RuleNodeInfo returns diagnostic node information for a rule.
func (net *Network) RuleNodeInfo(ruleName string) *RuleNodeInfo {
	net.mu.RLock()
	defer net.mu.RUnlock()
	return net.ruleNodeInfos[ruleName]
}

// HasRule returns true if a rule with the specified name is compiled into the Rete network.
func (net *Network) HasRule(ruleName string) bool {
	net.mu.RLock()
	defer net.mu.RUnlock()
	_, exists := net.terminals[ruleName]
	return exists
}
