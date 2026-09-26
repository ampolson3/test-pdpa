package render

import "fmt"

// Block is one line of a document as a reader sees it (heading, paragraph, list item, clause title), with fields
// resolved and clauses expanded.
type Block struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// Blocks flattens a rendered input into blocks.
func Blocks(in Input) []Block {
	var out []Block
	var rec func(n Node, prefix string)
	rec = func(n Node, prefix string) {
		switch n.Type {
		case "paragraph":
			out = append(out, Block{"paragraph", prefix + PlainText(n, in.Fields)})
		case "heading":
			out = append(out, Block{fmt.Sprintf("heading%d", Level(n)), PlainText(n, in.Fields)})
		case "clauseBlock":
			if t := attr(n, "title"); t != "" {
				out = append(out, Block{"clause", t})
			}
			for _, c := range n.Content {
				rec(c, prefix)
			}
		case "bulletList", "orderedList":
			for i, item := range n.Content {
				marker := "• "
				if n.Type == "orderedList" {
					marker = fmt.Sprintf("%d. ", i+1)
				}
				for j, c := range item.Content {
					p := prefix + "  "
					if j == 0 {
						p = prefix + marker
					}
					rec(c, p)
				}
			}
		case "horizontalRule":
			out = append(out, Block{"rule", "—"})
		case "clause":
			out = append(out, Block{"clause", fmt.Sprintf("[%s@%d]", attr(n, "code"), ClauseVersion(n))})
		default:
			for _, c := range n.Content {
				rec(c, prefix)
			}
		}
	}
	rec(in.expanded(), "")
	return out
}

// Change is one step of a comparison: equal, insert, delete, or change (a block edited in place, with the text
// differences inside it).
type Change struct {
	Op       string    `json:"op"`
	Kind     string    `json:"kind"`
	Before   string    `json:"before,omitempty"`
	After    string    `json:"after,omitempty"`
	Segments []Segment `json:"segments,omitempty"`
}

// Segment is part of a changed block: equal, insert or delete text.
type Segment struct {
	Op   string `json:"op"`
	Text string `json:"text"`
}

// Compare lines up two versions block by block (longest common subsequence); a deletion directly followed by an
// insertion of the same kind is reported as a change with character-level segments (Thai has no spaces between
// words, so words aren't a useful unit).
func Compare(a, b []Block) []Change {
	n, m := len(a), len(b)
	if n*m > 4_000_000 { // pathological sizes: fall back to delete-all/insert-all
		var out []Change
		for _, x := range a {
			out = append(out, Change{Op: "delete", Kind: x.Kind, Before: x.Text})
		}
		for _, y := range b {
			out = append(out, Change{Op: "insert", Kind: y.Kind, After: y.Text})
		}
		return out
	}
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var raw []Change
	i, j := 0, 0
	for i < n || j < m {
		switch {
		case i < n && j < m && a[i] == b[j]:
			raw = append(raw, Change{Op: "equal", Kind: a[i].Kind, Before: a[i].Text, After: b[j].Text})
			i++
			j++
		case i < n && (j == m || lcs[i+1][j] >= lcs[i][j+1]):
			raw = append(raw, Change{Op: "delete", Kind: a[i].Kind, Before: a[i].Text})
			i++
		default:
			raw = append(raw, Change{Op: "insert", Kind: b[j].Kind, After: b[j].Text})
			j++
		}
	}
	// Pair runs of deletions with the insertions that follow them.
	var out []Change
	for k := 0; k < len(raw); {
		if raw[k].Op != "delete" {
			out = append(out, raw[k])
			k++
			continue
		}
		var dels, ins []Change
		for k < len(raw) && raw[k].Op == "delete" {
			dels = append(dels, raw[k])
			k++
		}
		for k < len(raw) && raw[k].Op == "insert" {
			ins = append(ins, raw[k])
			k++
		}
		// Each deleted block pairs with the most similar inserted block of the same kind after the previous pair
		// (order kept), when they share at least half their text; the rest stay deletions and insertions.
		next := 0
		for _, d := range dels {
			best, bestSim := -1, 0.5
			for q := next; q < len(ins); q++ {
				if ins[q].Kind != d.Kind {
					continue
				}
				if sim := similarity(d.Before, ins[q].After); sim >= bestSim {
					best, bestSim = q, sim
				}
			}
			if best < 0 {
				out = append(out, d)
				continue
			}
			out = append(out, ins[next:best]...)
			out = append(out, Change{Op: "change", Kind: d.Kind, Before: d.Before, After: ins[best].After, Segments: Inline(d.Before, ins[best].After)})
			next = best + 1
		}
		out = append(out, ins[next:]...)
	}
	return out
}

// similarity is 2·LCS / (len a + len b) over characters: 1 for equal texts, 0 for nothing in common.
func similarity(a, b string) float64 {
	x, y := []rune(a), []rune(b)
	if len(x)+len(y) == 0 {
		return 1
	}
	if len(x)*len(y) > 4_000_000 {
		return 0
	}
	prev, cur := make([]int, len(y)+1), make([]int, len(y)+1)
	for i := len(x) - 1; i >= 0; i-- {
		for j := len(y) - 1; j >= 0; j-- {
			if x[i] == y[j] {
				cur[j] = prev[j+1] + 1
			} else {
				cur[j] = max(prev[j], cur[j+1])
			}
		}
		prev, cur = cur, prev
	}
	return 2 * float64(prev[0]) / float64(len(x)+len(y))
}

// Inline is a character-level diff of two texts, merged into runs. Equal runs of one or two characters between
// changes (typically a lone Thai vowel or tone mark that two different words happen to share) are folded into the
// change around them, so a replaced word reads as one deletion and one insertion.
func Inline(before, after string) []Segment {
	return cleanup(inline(before, after))
}

func cleanup(segs []Segment) []Segment {
	for i := 1; i < len(segs)-1; i++ {
		if segs[i].Op == "equal" && len([]rune(segs[i].Text)) < 3 {
			segs[i] = Segment{"both", segs[i].Text}
		}
	}
	var out []Segment
	for i := 0; i < len(segs); {
		if segs[i].Op == "equal" {
			out = append(out, segs[i])
			i++
			continue
		}
		var del, ins string
		for ; i < len(segs) && segs[i].Op != "equal"; i++ {
			switch segs[i].Op {
			case "delete":
				del += segs[i].Text
			case "insert":
				ins += segs[i].Text
			case "both":
				del += segs[i].Text
				ins += segs[i].Text
			}
		}
		if del != "" {
			out = append(out, Segment{"delete", del})
		}
		if ins != "" {
			out = append(out, Segment{"insert", ins})
		}
	}
	return out
}

func inline(before, after string) []Segment {
	a, b := []rune(before), []rune(after)
	if len(a)*len(b) > 4_000_000 {
		return []Segment{{"delete", before}, {"insert", after}}
	}
	n, m := len(a), len(b)
	lcs := make([][]int32, n+1)
	for i := range lcs {
		lcs[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out []Segment
	add := func(op string, r rune) {
		if len(out) > 0 && out[len(out)-1].Op == op {
			out[len(out)-1].Text += string(r)
			return
		}
		out = append(out, Segment{op, string(r)})
	}
	i, j := 0, 0
	for i < n || j < m {
		switch {
		case i < n && j < m && a[i] == b[j]:
			add("equal", a[i])
			i++
			j++
		case i < n && (j == m || lcs[i+1][j] >= lcs[i][j+1]):
			add("delete", a[i])
			i++
		default:
			add("insert", b[j])
			j++
		}
	}
	return out
}

// Summary counts a comparison's changes by op.
func Summary(cs []Change) map[string]int {
	out := map[string]int{}
	for _, c := range cs {
		out[c.Op]++
	}
	return out
}
