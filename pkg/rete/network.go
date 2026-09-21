package rete

import (
	"fmt"
	"sync"

	"ops5/pkg/model"
)

type terminalInfo struct {
	terminal *TerminalNode
	parent   interface {
		RemoveSuccessor(node LeftActivatable)
	}
}

// Network coordinates the Alpha and Beta networks and compiles rules into Rete nodes.
type Network struct {
	mu           sync.RWMutex
	alphaRoot    *AlphaRootNode
	rootBetaMem  *BetaMemory
	alphaMemPool map[string]*AlphaMemory
	terminals    map[string]terminalInfo
}

// NewNetwork creates an initialized Rete network.
func NewNetwork() *Network {
	net := &Network{
		alphaRoot:    NewAlphaRootNode(),
		rootBetaMem:  NewBetaMemory(),
		alphaMemPool: make(map[string]*AlphaMemory),
		terminals:    make(map[string]terminalInfo),
	}
	// Seed the root BetaMemory with the dummy token
	net.rootBetaMem.LeftActivation(DummyRootToken(), TagAdd)
	return net
}

// OnAssert implements wm.Listener to route WME assertions into the alpha network.
func (net *Network) OnAssert(wme *model.WME) {
	net.alphaRoot.Activation(wme, TagAdd)
}

// OnRetract implements wm.Listener to route WME retractions into the alpha network.
func (net *Network) OnRetract(wme *model.WME) {
	net.alphaRoot.Activation(wme, TagRemove)
}

// getAlphaKey produces a canonical key for sharing AlphaMemory across equivalent conditions.
func getAlphaKey(ce *model.ConditionElement) string {
	key := ce.Class
	for _, at := range ce.Tests {
		isMulti := len(at.Constraints) > 1
		for idx, c := range at.Constraints {
			if !c.Value.IsVariable() {
				if isMulti {
					key += fmt.Sprintf("|%s[%d]%s%s", at.Attribute, idx, c.Op.String(), c.Value.String())
				} else {
					key += fmt.Sprintf("|%s%s%s", at.Attribute, c.Op.String(), c.Value.String())
				}
			}
		}
	}
	return key
}

func matchesCEConstants(ce *model.ConditionElement, wme *model.WME) bool {
	if ce.Class != "*" && ce.Class != wme.Class {
		return false
	}
	for _, at := range ce.Tests {
		isMulti := len(at.Constraints) > 1
		for idx, c := range at.Constraints {
			if !c.Value.IsVariable() {
				vecIdx := -1
				if isMulti {
					vecIdx = idx
				}
				ct := NewIndexedConstantTestNode(at.Attribute, c.Op, c.Value, vecIdx)
				if !ct.Test(wme) {
					return false
				}
			}
		}
	}
	return true
}

// buildAlphaMemory creates or retrieves a shared AlphaMemory for the given condition element.
func (net *Network) buildAlphaMemory(ce *model.ConditionElement, existingWMEs []*model.WME) *AlphaMemory {
	key := getAlphaKey(ce)
	if am, exists := net.alphaMemPool[key]; exists {
		return am
	}

	currNode := AlphaNode(net.alphaRoot.GetOrCreateTypeNode(ce.Class))

	// Chain constant test nodes
	for _, at := range ce.Tests {
		isMulti := len(at.Constraints) > 1
		for idx, c := range at.Constraints {
			if !c.Value.IsVariable() {
				vecIdx := -1
				if isMulti {
					vecIdx = idx
				}
				testNode := NewIndexedConstantTestNode(at.Attribute, c.Op, c.Value, vecIdx)
				switch p := currNode.(type) {
				case *TypeNode:
					p.AddSuccessor(testNode)
				case *ConstantTestNode:
					p.AddSuccessor(testNode)
				}
				currNode = testNode
			}
		}
	}

	am := NewAlphaMemory()
	// Pre-populate with matching existing WMEs
	for _, wme := range existingWMEs {
		if matchesCEConstants(ce, wme) {
			am.items[wme.Timetag] = wme
		}
	}

	switch p := currNode.(type) {
	case *TypeNode:
		p.AddSuccessor(am)
	case *ConstantTestNode:
		p.AddSuccessor(am)
	}

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

	// If rule already exists in Rete network, remove previous terminal
	if prev, exists := net.terminals[rule.Name]; exists {
		prev.terminal.Deactivate()
		if prev.parent != nil {
			prev.parent.RemoveSuccessor(prev.terminal)
		}
		delete(net.terminals, rule.Name)
	}

	boundVariables := make(map[string]bool)
	currBetaMem := net.rootBetaMem

	for i, ce := range rule.Conditions {
		isLast := (i == len(rule.Conditions)-1)

		if ce.IsTest {
			var nextBetaNode LeftActivatable
			var terminal *TerminalNode

			if isLast {
				terminal = NewTerminalNode(rule, listener)
				nextBetaNode = terminal
			} else {
				nextBetaMem := NewBetaMemory()
				nextBetaNode = nextBetaMem
			}

			evalNode := NewEvalNode(ce.EvalTest)
			evalNode.AddSuccessor(nextBetaNode)
			currBetaMem.AddSuccessor(evalNode)

			if isLast {
				net.terminals[rule.Name] = terminalInfo{
					terminal: terminal,
					parent:   evalNode,
				}
			} else {
				currBetaMem = nextBetaNode.(*BetaMemory)
			}
			continue
		}

		if ce.IsNCC {
			subConditions := ce.NCCConditions
			partner := NewNccPartnerNode(len(subConditions))
			nccNode := NewNccNode(currBetaMem, partner, ce)

			subBetaMem := currBetaMem
			subBoundVars := make(map[string]bool)
			for k, v := range boundVariables {
				subBoundVars[k] = v
			}

			for subIdx, subCE := range subConditions {
				subIsLast := (subIdx == len(subConditions)-1)

				var subNextNode LeftActivatable
				if subIsLast {
					subNextNode = partner
				} else {
					subNextBetaMem := NewBetaMemory()
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
						if c.Value.IsVariable() {
							varName := c.Value.VariableName()
							if subBoundVars[varName] {
								vecIdx := -1
								if isMulti {
									vecIdx = idx
								}
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
					if subCE.Accumulate != nil && subCE.Accumulate.ResultVar != "" {
						subBoundVars[subCE.Accumulate.ResultVar] = true
					}
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

			var nextBetaNode LeftActivatable
			var terminal *TerminalNode

			if isLast {
				terminal = NewTerminalNode(rule, listener)
				nextBetaNode = terminal
			} else {
				nextBetaMem := NewBetaMemory()
				nextBetaNode = nextBetaMem
			}

			nccNode.AddSuccessor(nextBetaNode)
			currBetaMem.AddSuccessor(nccNode)

			if isLast {
				net.terminals[rule.Name] = terminalInfo{
					terminal: terminal,
					parent:   nccNode,
				}
			} else {
				currBetaMem = nextBetaNode.(*BetaMemory)
			}
			continue
		}

		alphaMem := net.buildAlphaMemory(ce, existingWMEs)

		// Determine join tests: compare right WME attributes against variables already bound in previous CEs
		var joinTests []JoinTest
		for _, at := range ce.Tests {
			isMulti := len(at.Constraints) > 1
			for idx, c := range at.Constraints {
				if c.Value.IsVariable() {
					varName := c.Value.VariableName()
					if boundVariables[varName] {
						vecIdx := -1
						if isMulti {
							vecIdx = idx
						}
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

		var nextBetaNode LeftActivatable
		var terminal *TerminalNode

		if isLast {
			terminal = NewTerminalNode(rule, listener)
			nextBetaNode = terminal
		} else {
			nextBetaMem := NewBetaMemory()
			nextBetaNode = nextBetaMem
		}

		var parentNode interface {
			RemoveSuccessor(node LeftActivatable)
		}

		if ce.IsNegative {
			negNode := NewNegativeJoinNode(currBetaMem, alphaMem, ce, joinTests)
			negNode.AddSuccessor(nextBetaNode)
			negNode.Attach()
			parentNode = negNode
		} else if ce.IsExistential {
			existNode := NewExistentialJoinNode(currBetaMem, alphaMem, ce, joinTests)
			existNode.AddSuccessor(nextBetaNode)
			existNode.Attach()
			parentNode = existNode
		} else if ce.IsAccumulate {
			accNode := NewAccumulateNode(currBetaMem, alphaMem, ce, ce.Accumulate, joinTests)
			accNode.AddSuccessor(nextBetaNode)
			accNode.Attach()
			parentNode = accNode
		} else {
			joinNode := NewJoinNode(currBetaMem, alphaMem, ce, joinTests)
			joinNode.AddSuccessor(nextBetaNode)
			joinNode.Attach()
			parentNode = joinNode
		}

		if isLast {
			net.terminals[rule.Name] = terminalInfo{
				terminal: terminal,
				parent:   parentNode,
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

		if !isLast {
			currBetaMem = nextBetaNode.(*BetaMemory)
		}
	}
}

// RemoveRule detaches and deactivates the terminal node for the specified rule.
// Returns true if the rule was registered in the network and removed, false otherwise.
func (net *Network) RemoveRule(ruleName string) bool {
	net.mu.Lock()
	defer net.mu.Unlock()

	info, ok := net.terminals[ruleName]
	if !ok {
		return false
	}

	info.terminal.Deactivate()
	if info.parent != nil {
		info.parent.RemoveSuccessor(info.terminal)
	}
	delete(net.terminals, ruleName)
	return true
}

// HasRule returns true if a rule with the specified name is compiled into the Rete network.
func (net *Network) HasRule(ruleName string) bool {
	net.mu.RLock()
	defer net.mu.RUnlock()
	_, exists := net.terminals[ruleName]
	return exists
}
