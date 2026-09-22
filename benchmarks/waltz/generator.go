package waltz

import (
	"bufio"
	"os"
	"strings"
)

const waltzRules = `;;; Waltz Line Labeling Benchmark Problem
;;; Reference: https://github.com/johanlindberg/ops5

(literalize stage value)
(literalize line p1 p2)
(literalize edge p1 p2 joined label)
(literalize junction base p1 p2 p3 type)

(p reverse_edges
   (stage ^value duplicate)
   (line ^p1 <p1> ^p2 <p2>)
-->
   (make edge ^p1 <p1> ^p2 <p2> ^joined false ^label unknown)
   (make edge ^p1 <p2> ^p2 <p1> ^joined false ^label unknown)
   (remove 2)
)

(p done_reversing
   (stage ^value duplicate)
   -(line)
-->
   (modify 1 ^value detect_junctions)
)

(p make_4_junction
   (stage ^value detect_junctions)
   (edge ^p1 <base> ^p2 <p1> ^joined false)
   (edge ^p1 <base> ^p2 {<p2> <> <p1>} ^joined false)
   (edge ^p1 <base> ^p2 {<p3> <> <p1> <> <p2>} ^joined false)
   (edge ^p1 <base> ^p2 {<p4> <> <p1> <> <p2> <> <p3>} ^joined false)
-->
   (make junction ^base <base> ^p1 <p1> ^p2 <p2> ^p3 <p3> ^type 4j)
   (modify 2 ^joined true)
   (modify 3 ^joined true)
   (modify 4 ^joined true)
   (modify 5 ^joined true)
)

(p make_3_junction
   (stage ^value detect_junctions)
   (edge ^p1 <base> ^p2 <p1> ^joined false)
   (edge ^p1 <base> ^p2 {<p2> <> <p1>} ^joined false)
   (edge ^p1 <base> ^p2 {<p3> <> <p1> <> <p2>} ^joined false)
-->
   (make junction ^base <base> ^p1 <p1> ^p2 <p2> ^p3 <p3> ^type 3j)
   (modify 2 ^joined true)
   (modify 3 ^joined true)
   (modify 4 ^joined true)
)

(p make_2_junction
   (stage ^value detect_junctions)
   (edge ^p1 <base> ^p2 <p1> ^joined false)
   (edge ^p1 <base> ^p2 {<p2> <> <p1>} ^joined false)
   -(edge ^p1 <base> ^p2 {<> <p1> <> <p2>})
-->
   (make junction ^base <base> ^p1 <p1> ^p2 <p2> ^type 2j)
   (modify 2 ^joined true)
   (modify 3 ^joined true)
)

(p make_1_junction
   (stage ^value detect_junctions)
   (edge ^p1 <base> ^p2 <p1> ^joined false)
   -(edge ^p1 <base> ^p2 {<> <p1>})
-->
   (make junction ^base <base> ^p1 <p1> ^type 1j)
   (modify 2 ^joined true)
)

(p done_detecting
   (stage ^value detect_junctions)
   -(edge ^joined false)
-->
   (modify 1 ^value find_boundary)
)

(p seed_boundary
   (stage ^value find_boundary)
   (junction ^base <b1> ^p1 <p1> ^type 2j)
   {(edge ^p1 <b1> ^p2 <p1> ^label unknown) <e>}
-->
   (modify <e> ^label boundary)
   (modify 1 ^value propagate)
)

(p fallback_boundary
   (stage ^value find_boundary)
   {(edge ^label unknown) <e>}
-->
   (modify <e> ^label boundary)
   (modify 1 ^value propagate)
)

(p edge_reciprocity
   (stage ^value propagate)
   (edge ^p1 <p1> ^p2 <p2> ^label {<lbl> <> unknown})
   {(edge ^p1 <p2> ^p2 <p1> ^label unknown) <opp>}
-->
   (modify <opp> ^label <lbl>)
)

(p propagate_2j_boundary_1
   (stage ^value propagate)
   (junction ^base <b> ^p1 <p1> ^p2 <p2> ^type 2j)
   (edge ^p1 <b> ^p2 <p1> ^label boundary)
   {(edge ^p1 <b> ^p2 <p2> ^label unknown) <e2>}
-->
   (modify <e2> ^label boundary)
)

(p propagate_2j_boundary_2
   (stage ^value propagate)
   (junction ^base <b> ^p1 <p1> ^p2 <p2> ^type 2j)
   (edge ^p1 <b> ^p2 <p2> ^label boundary)
   {(edge ^p1 <b> ^p2 <p1> ^label unknown) <e1>}
-->
   (modify <e1> ^label boundary)
)

(p propagate_2j_convex_1
   (stage ^value propagate)
   (junction ^base <b> ^p1 <p1> ^p2 <p2> ^type 2j)
   (edge ^p1 <b> ^p2 <p1> ^label +)
   {(edge ^p1 <b> ^p2 <p2> ^label unknown) <e2>}
-->
   (modify <e2> ^label +)
)

(p propagate_2j_convex_2
   (stage ^value propagate)
   (junction ^base <b> ^p1 <p1> ^p2 <p2> ^type 2j)
   (edge ^p1 <b> ^p2 <p2> ^label +)
   {(edge ^p1 <b> ^p2 <p1> ^label unknown) <e1>}
-->
   (modify <e1> ^label +)
)

(p propagate_3j_fork_inner_1
   (stage ^value propagate)
   (junction ^base <b> ^p1 <p1> ^p2 <p2> ^p3 <p3> ^type 3j)
   (edge ^p1 <b> ^p2 <p1> ^label boundary)
   (edge ^p1 <b> ^p2 <p2> ^label boundary)
   {(edge ^p1 <b> ^p2 <p3> ^label unknown) <e3>}
-->
   (modify <e3> ^label +)
)

(p propagate_3j_fork_inner_2
   (stage ^value propagate)
   (junction ^base <b> ^p1 <p1> ^p2 <p2> ^p3 <p3> ^type 3j)
   (edge ^p1 <b> ^p2 <p1> ^label boundary)
   (edge ^p1 <b> ^p2 <p3> ^label boundary)
   {(edge ^p1 <b> ^p2 <p2> ^label unknown) <e2>}
-->
   (modify <e2> ^label +)
)

(p propagate_3j_fork_inner_3
   (stage ^value propagate)
   (junction ^base <b> ^p1 <p1> ^p2 <p2> ^p3 <p3> ^type 3j)
   (edge ^p1 <b> ^p2 <p2> ^label boundary)
   (edge ^p1 <b> ^p2 <p3> ^label boundary)
   {(edge ^p1 <b> ^p2 <p1> ^label unknown) <e1>}
-->
   (modify <e1> ^label +)
)

(p propagate_3j_all_convex_1
   (stage ^value propagate)
   (junction ^base <b> ^p1 <p1> ^p2 <p2> ^p3 <p3> ^type 3j)
   (edge ^p1 <b> ^p2 <p1> ^label +)
   (edge ^p1 <b> ^p2 <p2> ^label +)
   {(edge ^p1 <b> ^p2 <p3> ^label unknown) <e3>}
-->
   (modify <e3> ^label +)
)

(p propagate_3j_all_convex_2
   (stage ^value propagate)
   (junction ^base <b> ^p1 <p1> ^p2 <p2> ^p3 <p3> ^type 3j)
   (edge ^p1 <b> ^p2 <p1> ^label +)
   (edge ^p1 <b> ^p2 <p3> ^label +)
   {(edge ^p1 <b> ^p2 <p2> ^label unknown) <e2>}
-->
   (modify <e2> ^label +)
)

(p propagate_3j_all_convex_3
   (stage ^value propagate)
   (junction ^base <b> ^p1 <p1> ^p2 <p2> ^p3 <p3> ^type 3j)
   (edge ^p1 <b> ^p2 <p2> ^label +)
   (edge ^p1 <b> ^p2 <p3> ^label +)
   {(edge ^p1 <b> ^p2 <p1> ^label unknown) <e1>}
-->
   (modify <e1> ^label +)
)

(p default_edge_label
   (stage ^value propagate)
   {(edge ^label unknown) <e>}
-->
   (modify <e> ^label -)
)

(p labeling_done
   (stage ^value propagate)
   -(edge ^label unknown)
-->
   (write "Waltz labeling complete!" (crlf))
   (modify 1 ^value done)
   (halt)
)
`

// BuildWaltzOps combines the waltz rules with extracted line data.
func BuildWaltzOps(lineDataFile string, outputFile string) error {
	f, err := os.Open(lineDataFile)
	if err != nil {
		return err
	}
	defer f.Close()

	var sb strings.Builder
	sb.WriteString(waltzRules)
	sb.WriteString("\n\n(make stage ^value duplicate)\n")

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "(make line ") && strings.HasSuffix(line, ")") {
			sb.WriteString(line)
			sb.WriteString("\n")
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	return os.WriteFile(outputFile, []byte(sb.String()), 0644)
}
