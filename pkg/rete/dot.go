package rete

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// ExportDOT serializes the compiled Rete network topology into Graphviz DOT language format.
func (net *Network) ExportDOT(w io.Writer) error {
	net.mu.RLock()
	defer net.mu.RUnlock()
	return exportNetworkDOT(net, w)
}

type dotWriter struct {
	w            io.Writer
	alphaMemKeys map[*AlphaMemory]string
	alphaMemIDs  map[*AlphaMemory]string
	switchNodeIDs map[*AlphaSwitchNode]string
	testNodeIDs  map[*ConstantTestNode]string
	typeNodeIDs  map[*TypeNode]string
	betaMemIDs   map[*BetaMemory]string
	betaNodeIDs  map[LeftActivatable]string
	terminalIDs  map[*TerminalNode]string

	nextAlphaMemID   int
	nextSwitchNodeID int
	nextTestNodeID   int
	nextBetaNodeID   int

	alphaNodes []string
	alphaEdges []string
	betaNodes  []string
	betaEdges  []string
	crossEdges []string
}

func newDOTWriter(w io.Writer) *dotWriter {
	return &dotWriter{
		w:             w,
		alphaMemKeys:  make(map[*AlphaMemory]string),
		alphaMemIDs:   make(map[*AlphaMemory]string),
		switchNodeIDs: make(map[*AlphaSwitchNode]string),
		testNodeIDs:   make(map[*ConstantTestNode]string),
		typeNodeIDs:   make(map[*TypeNode]string),
		betaMemIDs:    make(map[*BetaMemory]string),
		betaNodeIDs:   make(map[LeftActivatable]string),
		terminalIDs:   make(map[*TerminalNode]string),
	}
}

func escapeDOT(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}

func sanitizeID(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

func (d *dotWriter) getOrCreateAlphaMemID(am *AlphaMemory) string {
	if id, ok := d.alphaMemIDs[am]; ok {
		return id
	}
	id := fmt.Sprintf("am_%d", d.nextAlphaMemID)
	d.nextAlphaMemID++
	d.alphaMemIDs[am] = id
	return id
}

func (d *dotWriter) getOrCreateSwitchNodeID(asn *AlphaSwitchNode) string {
	if id, ok := d.switchNodeIDs[asn]; ok {
		return id
	}
	id := fmt.Sprintf("asn_%d", d.nextSwitchNodeID)
	d.nextSwitchNodeID++
	d.switchNodeIDs[asn] = id
	return id
}

func (d *dotWriter) getOrCreateTestNodeID(ctn *ConstantTestNode) string {
	if id, ok := d.testNodeIDs[ctn]; ok {
		return id
	}
	id := fmt.Sprintf("ctn_%d", d.nextTestNodeID)
	d.nextTestNodeID++
	d.testNodeIDs[ctn] = id
	return id
}

func (d *dotWriter) getOrCreateBetaMemID(bm *BetaMemory) string {
	if id, ok := d.betaMemIDs[bm]; ok {
		return id
	}
	id := fmt.Sprintf("bm_%d", bm.id)
	d.betaMemIDs[bm] = id
	return id
}

func (d *dotWriter) getOrCreateBetaNodeID(node LeftActivatable) string {
	if bm, ok := node.(*BetaMemory); ok {
		return d.getOrCreateBetaMemID(bm)
	}
	if tn, ok := node.(*TerminalNode); ok {
		return d.getOrCreateTerminalID(tn)
	}
	if id, ok := d.betaNodeIDs[node]; ok {
		return id
	}
	var prefix string
	switch node.(type) {
	case *JoinNode:
		prefix = "jn"
	case *NegativeJoinNode:
		prefix = "njn"
	case *ExistentialJoinNode:
		prefix = "ejn"
	case *AccumulateNode:
		prefix = "acc"
	case *EvalNode:
		prefix = "eval"
	case *NccNode:
		prefix = "ncc"
	case *NccPartnerNode:
		prefix = "ncc_partner"
	default:
		prefix = "bnode"
	}
	id := fmt.Sprintf("%s_%d", prefix, d.nextBetaNodeID)
	d.nextBetaNodeID++
	d.betaNodeIDs[node] = id
	return id
}

func (d *dotWriter) getOrCreateTerminalID(tn *TerminalNode) string {
	if id, ok := d.terminalIDs[tn]; ok {
		return id
	}
	id := fmt.Sprintf("term_%s", sanitizeID(tn.rule.Name))
	d.terminalIDs[tn] = id
	return id
}

func formatConstantTest(ctn *ConstantTestNode) string {
	if len(ctn.Disjunction) > 0 {
		var parts []string
		for _, dj := range ctn.Disjunction {
			parts = append(parts, fmt.Sprintf("%s %s", dj.Op.String(), dj.Value.String()))
		}
		if ctn.VectorIndex >= 0 {
			return fmt.Sprintf("^%s[%d] << %s >>", ctn.Attribute, ctn.VectorIndex, strings.Join(parts, " "))
		}
		return fmt.Sprintf("^%s << %s >>", ctn.Attribute, strings.Join(parts, " "))
	}
	if ctn.VectorIndex >= 0 {
		return fmt.Sprintf("^%s[%d] %s %s", ctn.Attribute, ctn.VectorIndex, ctn.Op.String(), ctn.Value.String())
	}
	return fmt.Sprintf("^%s %s %s", ctn.Attribute, ctn.Op.String(), ctn.Value.String())
}

func formatJoinTests(tests []JoinTest) string {
	if len(tests) == 0 {
		return "(no join tests)"
	}
	var parts []string
	for _, jt := range tests {
		if len(jt.Disjunction) > 0 {
			var djParts []string
			for _, dj := range jt.Disjunction {
				djParts = append(djParts, fmt.Sprintf("%s %s", dj.Op.String(), dj.Value.String()))
			}
			if jt.VectorIndex >= 0 {
				parts = append(parts, fmt.Sprintf("^%s[%d] << %s >>", jt.Attribute, jt.VectorIndex, strings.Join(djParts, " ")))
			} else {
				parts = append(parts, fmt.Sprintf("^%s << %s >>", jt.Attribute, strings.Join(djParts, " ")))
			}
		} else {
			if jt.VectorIndex >= 0 {
				parts = append(parts, fmt.Sprintf("^%s[%d] %s <%s>", jt.Attribute, jt.VectorIndex, jt.Op.String(), jt.Variable))
			} else {
				parts = append(parts, fmt.Sprintf("^%s %s <%s>", jt.Attribute, jt.Op.String(), jt.Variable))
			}
		}
	}
	return strings.Join(parts, "\n")
}

func formatTerminalNode(tn *TerminalNode) string {
	count := tn.ActivationCount()
	if tn.rule.Salience != 0 {
		return fmt.Sprintf("Rule: %s\n[salience: %d]\nActivations: %d", tn.rule.Name, tn.rule.Salience, count)
	}
	return fmt.Sprintf("Rule: %s\nActivations: %d", tn.rule.Name, count)
}

func (d *dotWriter) traverseAlphaSuccessors(parentID string, succs []AlphaNode, edgeLabel string) {
	for _, succ := range succs {
		switch node := succ.(type) {
		case *AlphaSwitchNode:
			id := d.getOrCreateSwitchNodeID(node)
			label := fmt.Sprintf("Switch: ^%s", node.Attribute)
			if node.VectorIndex >= 0 {
				label = fmt.Sprintf("Switch: ^%s[%d]", node.Attribute, node.VectorIndex)
			}
			d.alphaNodes = append(d.alphaNodes, fmt.Sprintf("    %s [label=\"%s\", shape=hexagon, style=\"filled,rounded\", fillcolor=\"#ede7f6\", color=\"#512da8\", fontcolor=\"#311b92\"];",
				id, escapeDOT(label)))
			if edgeLabel != "" {
				d.alphaEdges = append(d.alphaEdges, fmt.Sprintf("    %s -> %s [label=\"%s\", color=\"#512da8\", fontcolor=\"#512da8\"];", parentID, id, escapeDOT(edgeLabel)))
			} else {
				d.alphaEdges = append(d.alphaEdges, fmt.Sprintf("    %s -> %s [color=\"#512da8\"];", parentID, id))
			}

			node.mu.RLock()
			var keys []string
			for k := range node.cases {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				branchSuccs := append([]AlphaNode(nil), node.cases[k]...)
				d.traverseAlphaSuccessors(id, branchSuccs, k)
			}
			node.mu.RUnlock()

		case *ConstantTestNode:
			id := d.getOrCreateTestNodeID(node)
			label := formatConstantTest(node)
			d.alphaNodes = append(d.alphaNodes, fmt.Sprintf("    %s [label=\"%s\", shape=box, style=\"filled,rounded\", fillcolor=\"#e1f5fe\", color=\"#0288d1\", fontcolor=\"#014361\"];",
				id, escapeDOT(label)))
			if edgeLabel != "" {
				d.alphaEdges = append(d.alphaEdges, fmt.Sprintf("    %s -> %s [label=\"%s\", color=\"#0288d1\", fontcolor=\"#0288d1\"];", parentID, id, escapeDOT(edgeLabel)))
			} else {
				d.alphaEdges = append(d.alphaEdges, fmt.Sprintf("    %s -> %s [color=\"#0288d1\"];", parentID, id))
			}
			d.traverseAlphaSuccessors(id, node.successors, "")

		case *AlphaMemory:
			id := d.getOrCreateAlphaMemID(node)
			key := d.alphaMemKeys[node]
			var label string
			if key != "" {
				label = fmt.Sprintf("AlphaMemory\n(%s)\nItems: %d", key, node.ItemCount())
			} else {
				label = fmt.Sprintf("AlphaMemory\nItems: %d", node.ItemCount())
			}
			d.alphaNodes = append(d.alphaNodes, fmt.Sprintf("    %s [label=\"%s\", shape=box, style=\"filled,rounded\", fillcolor=\"#e8f0fe\", color=\"#4285f4\", fontcolor=\"#174ea6\"];",
				id, escapeDOT(label)))
			if edgeLabel != "" {
				d.alphaEdges = append(d.alphaEdges, fmt.Sprintf("    %s -> %s [label=\"%s\", color=\"#4285f4\", fontcolor=\"#4285f4\"];", parentID, id, escapeDOT(edgeLabel)))
			} else {
				d.alphaEdges = append(d.alphaEdges, fmt.Sprintf("    %s -> %s [color=\"#4285f4\"];", parentID, id))
			}
		}
	}
}

func exportNetworkDOT(net *Network, w io.Writer) error {
	d := newDOTWriter(w)

	// Map AlphaMemories to their canonical keys
	for k, am := range net.alphaMemPool {
		d.alphaMemKeys[am] = k
	}

	// -------------------------------------------------------------
	// 1. Alpha Network Traversal
	// -------------------------------------------------------------
	d.alphaNodes = append(d.alphaNodes, `    alpha_root [label="Alpha Root", shape=octagon, style=filled, fillcolor="#cfe2f3", color="#1c4587", fontcolor="#0d2346"];`)

	var classes []string
	for c := range net.alphaRoot.typeNodes {
		classes = append(classes, c)
	}
	sort.Strings(classes)

	for _, c := range classes {
		tn := net.alphaRoot.typeNodes[c]
		typeID := fmt.Sprintf("type_%s", sanitizeID(c))
		d.typeNodeIDs[tn] = typeID
		d.alphaNodes = append(d.alphaNodes, fmt.Sprintf("    %s [label=\"Type: %s\", shape=octagon, style=filled, fillcolor=\"#d9e8fb\", color=\"#2a62a8\", fontcolor=\"#0d2346\"];",
			typeID, escapeDOT(c)))
		d.alphaEdges = append(d.alphaEdges, fmt.Sprintf("    alpha_root -> %s [color=\"#2a62a8\"];", typeID))
		d.traverseAlphaSuccessors(typeID, tn.successors, "")
	}

	// -------------------------------------------------------------
	// 2. Beta Network Traversal
	// -------------------------------------------------------------
	rootID := d.getOrCreateBetaMemID(net.rootBetaMem)
	rootLabel := fmt.Sprintf("Root BetaMemory (bm0)\nTokens: %d", net.rootBetaMem.TokenCount())
	d.betaNodes = append(d.betaNodes, fmt.Sprintf("    %s [label=\"%s\", shape=box, style=\"filled,rounded\", fillcolor=\"#e6f4ea\", color=\"#137333\", fontcolor=\"#0d4d22\"];",
		rootID, escapeDOT(rootLabel)))

	queue := []LeftActivatable{net.rootBetaMem}
	visited := make(map[LeftActivatable]bool)
	visited[net.rootBetaMem] = true

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		switch node := curr.(type) {
		case *BetaMemory:
			bmID := d.getOrCreateBetaMemID(node)
			if node != net.rootBetaMem {
				label := fmt.Sprintf("BetaMemory (bm%d)\nTokens: %d", node.id, node.TokenCount())
				d.betaNodes = append(d.betaNodes, fmt.Sprintf("    %s [label=\"%s\", shape=box, style=\"filled,rounded\", fillcolor=\"#e6f4ea\", color=\"#137333\", fontcolor=\"#0d4d22\"];",
					bmID, escapeDOT(label)))
			}
			for _, succ := range node.successors {
				succID := d.getOrCreateBetaNodeID(succ)
				if _, isTerm := succ.(*TerminalNode); isTerm {
					d.betaEdges = append(d.betaEdges, fmt.Sprintf("    %s -> %s [label=\"activate\", style=solid, color=\"#8430ce\", fontcolor=\"#8430ce\"];", bmID, succID))
				} else {
					d.betaEdges = append(d.betaEdges, fmt.Sprintf("    %s -> %s [label=\"left\", style=solid, color=\"#137333\", fontcolor=\"#137333\"];", bmID, succID))
				}
				if !visited[succ] {
					visited[succ] = true
					queue = append(queue, succ)
				}
			}

		case *JoinNode:
			jnID := d.getOrCreateBetaNodeID(node)
			label := fmt.Sprintf("Join: (%s)\n%s", node.ce.Class, formatJoinTests(node.joinTests))
			d.betaNodes = append(d.betaNodes, fmt.Sprintf("    %s [label=\"%s\", shape=ellipse, style=filled, fillcolor=\"#feefe3\", color=\"#e8710a\", fontcolor=\"#8a3b00\"];",
				jnID, escapeDOT(label)))
			if node.alphaMemory != nil {
				amID := d.getOrCreateAlphaMemID(node.alphaMemory)
				d.crossEdges = append(d.crossEdges, fmt.Sprintf("  %s -> %s [label=\"right\", style=dashed, color=\"#4285f4\", fontcolor=\"#4285f4\"];", amID, jnID))
			}
			for _, succ := range node.successors {
				succID := d.getOrCreateBetaNodeID(succ)
				d.betaEdges = append(d.betaEdges, fmt.Sprintf("    %s -> %s [style=solid, color=\"#e8710a\"];", jnID, succID))
				if !visited[succ] {
					visited[succ] = true
					queue = append(queue, succ)
				}
			}

		case *NegativeJoinNode:
			njnID := d.getOrCreateBetaNodeID(node)
			label := fmt.Sprintf("NegativeJoin: -(%s)\n%s", node.ce.Class, formatJoinTests(node.joinTests))
			d.betaNodes = append(d.betaNodes, fmt.Sprintf("    %s [label=\"%s\", shape=ellipse, style=filled, fillcolor=\"#fce8e6\", color=\"#c5221f\", fontcolor=\"#78100e\"];",
				njnID, escapeDOT(label)))
			if node.alphaMemory != nil {
				amID := d.getOrCreateAlphaMemID(node.alphaMemory)
				d.crossEdges = append(d.crossEdges, fmt.Sprintf("  %s -> %s [label=\"right (neg)\", style=dashed, color=\"#c5221f\", fontcolor=\"#c5221f\"];", amID, njnID))
			}
			for _, succ := range node.successors {
				succID := d.getOrCreateBetaNodeID(succ)
				d.betaEdges = append(d.betaEdges, fmt.Sprintf("    %s -> %s [style=solid, color=\"#c5221f\"];", njnID, succID))
				if !visited[succ] {
					visited[succ] = true
					queue = append(queue, succ)
				}
			}

		case *ExistentialJoinNode:
			ejnID := d.getOrCreateBetaNodeID(node)
			label := fmt.Sprintf("ExistsJoin: (exists %s)\n%s", node.ce.Class, formatJoinTests(node.joinTests))
			d.betaNodes = append(d.betaNodes, fmt.Sprintf("    %s [label=\"%s\", shape=ellipse, style=filled, fillcolor=\"#fff0d4\", color=\"#b06000\", fontcolor=\"#6b3a00\"];",
				ejnID, escapeDOT(label)))
			if node.alphaMemory != nil {
				amID := d.getOrCreateAlphaMemID(node.alphaMemory)
				d.crossEdges = append(d.crossEdges, fmt.Sprintf("  %s -> %s [label=\"right (exists)\", style=dashed, color=\"#b06000\", fontcolor=\"#b06000\"];", amID, ejnID))
			}
			for _, succ := range node.successors {
				succID := d.getOrCreateBetaNodeID(succ)
				d.betaEdges = append(d.betaEdges, fmt.Sprintf("    %s -> %s [style=solid, color=\"#b06000\"];", ejnID, succID))
				if !visited[succ] {
					visited[succ] = true
					queue = append(queue, succ)
				}
			}

		case *AccumulateNode:
			accID := d.getOrCreateBetaNodeID(node)
			label := fmt.Sprintf("Accumulate: (%s)\n%s -> <%s>\n%s", node.ce.Class, node.spec.Op.String(), node.spec.ResultVar, formatJoinTests(node.joinTests))
			d.betaNodes = append(d.betaNodes, fmt.Sprintf("    %s [label=\"%s\", shape=ellipse, style=filled, fillcolor=\"#e8eaed\", color=\"#5f6368\", fontcolor=\"#202124\"];",
				accID, escapeDOT(label)))
			if node.alphaMemory != nil {
				amID := d.getOrCreateAlphaMemID(node.alphaMemory)
				d.crossEdges = append(d.crossEdges, fmt.Sprintf("  %s -> %s [label=\"right\", style=dashed, color=\"#5f6368\", fontcolor=\"#5f6368\"];", amID, accID))
			}
			for _, succ := range node.successors {
				succID := d.getOrCreateBetaNodeID(succ)
				d.betaEdges = append(d.betaEdges, fmt.Sprintf("    %s -> %s [style=solid, color=\"#5f6368\"];", accID, succID))
				if !visited[succ] {
					visited[succ] = true
					queue = append(queue, succ)
				}
			}

		case *EvalNode:
			evalID := d.getOrCreateBetaNodeID(node)
			var label string
			if node.test != nil {
				label = fmt.Sprintf("EvalNode\n%s", node.test.String())
			} else {
				label = "EvalNode"
			}
			d.betaNodes = append(d.betaNodes, fmt.Sprintf("    %s [label=\"%s\", shape=diamond, style=filled, fillcolor=\"#fef7e0\", color=\"#f9ab00\", fontcolor=\"#7a5500\"];",
				evalID, escapeDOT(label)))
			for _, succ := range node.successors {
				succID := d.getOrCreateBetaNodeID(succ)
				d.betaEdges = append(d.betaEdges, fmt.Sprintf("    %s -> %s [style=solid, color=\"#f9ab00\"];", evalID, succID))
				if !visited[succ] {
					visited[succ] = true
					queue = append(queue, succ)
				}
			}

		case *NccNode:
			nccID := d.getOrCreateBetaNodeID(node)
			d.betaNodes = append(d.betaNodes, fmt.Sprintf("    %s [label=\"%s\", shape=box, style=\"filled,rounded\", fillcolor=\"#fce8e6\", color=\"#c5221f\", fontcolor=\"#78100e\"];",
				nccID, escapeDOT("NccNode\n-(conjunction)")))
			if node.partner != nil {
				partnerID := d.getOrCreateBetaNodeID(node.partner)
				d.betaEdges = append(d.betaEdges, fmt.Sprintf("    %s -> %s [label=\"inhibit\", style=dotted, color=\"#c5221f\", fontcolor=\"#c5221f\"];", partnerID, nccID))
			}
			for _, succ := range node.successors {
				succID := d.getOrCreateBetaNodeID(succ)
				d.betaEdges = append(d.betaEdges, fmt.Sprintf("    %s -> %s [style=solid, color=\"#c5221f\"];", nccID, succID))
				if !visited[succ] {
					visited[succ] = true
					queue = append(queue, succ)
				}
			}

		case *NccPartnerNode:
			partnerID := d.getOrCreateBetaNodeID(node)
			d.betaNodes = append(d.betaNodes, fmt.Sprintf("    %s [label=\"NccPartner\", shape=box, style=\"filled,rounded\", fillcolor=\"#fce8e6\", color=\"#c5221f\", fontcolor=\"#78100e\"];",
				partnerID))

		case *TerminalNode:
			termID := d.getOrCreateTerminalID(node)
			label := formatTerminalNode(node)
			d.betaNodes = append(d.betaNodes, fmt.Sprintf("    %s [label=\"%s\", shape=doublecircle, style=filled, fillcolor=\"#f3e8fd\", color=\"#8430ce\", fontcolor=\"#491380\"];",
				termID, escapeDOT(label)))
		}
	}

	// Ensure any terminal nodes in net.terminals that were not encountered in BFS are added
	for _, ti := range net.terminals {
		if !visited[ti.terminal] {
			visited[ti.terminal] = true
			termID := d.getOrCreateTerminalID(ti.terminal)
			label := formatTerminalNode(ti.terminal)
			d.betaNodes = append(d.betaNodes, fmt.Sprintf("    %s [label=\"%s\", shape=doublecircle, style=filled, fillcolor=\"#f3e8fd\", color=\"#8430ce\", fontcolor=\"#491380\"];",
				termID, escapeDOT(label)))
			if bm, ok := ti.parent.(*BetaMemory); ok {
				bmID := d.getOrCreateBetaMemID(bm)
				d.betaEdges = append(d.betaEdges, fmt.Sprintf("    %s -> %s [label=\"activate\", style=solid, color=\"#8430ce\", fontcolor=\"#8430ce\"];", bmID, termID))
			}
		}
	}

	// -------------------------------------------------------------
	// 3. Render Complete Graph
	// -------------------------------------------------------------
	var b strings.Builder
	b.WriteString("digraph ReteNetwork {\n")
	b.WriteString("  rankdir=TB;\n")
	b.WriteString("  nodesep=0.4;\n")
	b.WriteString("  ranksep=0.6;\n")
	b.WriteString("  splines=true;\n")
	b.WriteString("  node [fontname=\"Helvetica,Arial,sans-serif\", fontsize=10];\n")
	b.WriteString("  edge [fontname=\"Helvetica,Arial,sans-serif\", fontsize=9];\n\n")

	// Alpha Network Cluster
	b.WriteString("  subgraph cluster_alpha {\n")
	b.WriteString("    label=\"Alpha Network\";\n")
	b.WriteString("    style=\"filled,dashed\";\n")
	b.WriteString("    color=\"#1c4587\";\n")
	b.WriteString("    fillcolor=\"#f8fafd\";\n")
	b.WriteString("    fontname=\"Helvetica,Arial,sans-serif\";\n")
	b.WriteString("    fontcolor=\"#1c4587\";\n")
	b.WriteString("    fontsize=12;\n\n")
	for _, n := range d.alphaNodes {
		b.WriteString(n + "\n")
	}
	b.WriteString("\n")
	for _, e := range d.alphaEdges {
		b.WriteString(e + "\n")
	}
	b.WriteString("  }\n\n")

	// Beta Network Cluster
	b.WriteString("  subgraph cluster_beta {\n")
	b.WriteString("    label=\"Beta Network\";\n")
	b.WriteString("    style=\"filled,dashed\";\n")
	b.WriteString("    color=\"#137333\";\n")
	b.WriteString("    fillcolor=\"#f8fdf9\";\n")
	b.WriteString("    fontname=\"Helvetica,Arial,sans-serif\";\n")
	b.WriteString("    fontcolor=\"#137333\";\n")
	b.WriteString("    fontsize=12;\n\n")
	for _, n := range d.betaNodes {
		b.WriteString(n + "\n")
	}
	b.WriteString("\n")
	for _, e := range d.betaEdges {
		b.WriteString(e + "\n")
	}
	b.WriteString("  }\n\n")

	// Cross-Network Edges (Alpha -> Beta)
	if len(d.crossEdges) > 0 {
		b.WriteString("  // Cross-Network Right Activations (Alpha -> Beta)\n")
		for _, e := range d.crossEdges {
			b.WriteString(e + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("}\n")

	_, err := io.WriteString(w, b.String())
	return err
}
