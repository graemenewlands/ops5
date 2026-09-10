package rete

import (
	"fmt"
	"sync"

	"ops5/pkg/model"
)

// Network coordinates the Alpha and Beta networks and compiles rules into Rete nodes.
type Network struct {
	mu           sync.RWMutex
	alphaRoot    *AlphaRootNode
	rootBetaMem  *BetaMemory
	alphaMemPool map[string]*AlphaMemory
}

// NewNetwork creates an initialized Rete network.
func NewNetwork() *Network {
	net := &Network{
		alphaRoot:    NewAlphaRootNode(),
		rootBetaMem:  NewBetaMemory(),
		alphaMemPool: make(map[string]*AlphaMemory),
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

	boundVariables := make(map[string]bool)
	currBetaMem := net.rootBetaMem

	for i, ce := range rule.Conditions {
		isLast := (i == len(rule.Conditions)-1)
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

		if isLast {
			terminal := NewTerminalNode(rule, listener)
			nextBetaNode = terminal
		} else {
			nextBetaMem := NewBetaMemory()
			nextBetaNode = nextBetaMem
		}

		if ce.IsNegative {
			negNode := NewNegativeJoinNode(currBetaMem, alphaMem, ce, joinTests)
			negNode.AddSuccessor(nextBetaNode)
			negNode.Attach()
		} else {
			joinNode := NewJoinNode(currBetaMem, alphaMem, ce, joinTests)
			joinNode.AddSuccessor(nextBetaNode)
			joinNode.Attach()
		}

		// Update bound variables for subsequent condition elements
		for _, v := range ce.Variables() {
			boundVariables[v] = true
		}

		if !isLast {
			currBetaMem = nextBetaNode.(*BetaMemory)
		}
	}
}
