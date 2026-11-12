// Flat command lists, pipelines, and left-associative arithmetic chains are
// built by loops in the parser. These tests specify how such chains interact
// with nesting accounting, and that the printer, Walk, and position accessors
// handle long chains without growing the call stack once per operator.
package syntax

import (
	"bufio"
	"io"
	"runtime"
	"strings"
	"testing"

	"github.com/go-quicktest/qt"
)

// flatChainLinks is the number of commands or terms in a long chain: far more
// than an embedding limit such as 128 units, and as many as the default limit.
const flatChainLinks = 10_000

// flatChain is a long chain together with its canonical printed form and the
// position of its root node, all written by hand from the source shape.
type flatChain struct {
	name    string
	src     string
	printer []PrinterOption
	want    string
	root    func(*File) Node
	// pos and end are the root's byte offsets; every chain is on line 1
	// unless multiLine is set.
	pos, end  uint
	multiLine bool
}

func commandChain(name, op string, n int) flatChain {
	src := "true" + strings.Repeat(op+"true", n-1) + "\n"
	return flatChain{
		name: name,
		src:  src,
		// The printer separates every list or pipeline operator with spaces.
		want: "true" + strings.Repeat(" "+op+" true", n-1) + "\n",
		root: func(f *File) Node { return f.Stmts[0].Cmd },
		pos:  0,
		end:  uint(len(src) - 1),
	}
}

func arithmChain(name, op string, n int) flatChain {
	expr := "1" + strings.Repeat(op+"1", n-1)
	return flatChain{
		name: name,
		src:  "echo $((" + expr + "))\n",
		// Arithmetic expansions print binary operators with surrounding spaces.
		want: "echo $((1" + strings.Repeat(" "+op+" 1", n-1) + "))\n",
		root: func(f *File) Node {
			return f.Stmts[0].Cmd.(*CallExpr).Args[1].Parts[0].(*ArithmExp).X
		},
		pos: uint(len("echo $((")),
		end: uint(len("echo $((") + len(expr)),
	}
}

func flatChains(n int) []flatChain {
	mixedOps := []string{"&&", "||", "|", "|&"}
	var mixedSrc, mixedWant strings.Builder
	mixedSrc.WriteString("true")
	mixedWant.WriteString("true")
	for i := range n - 1 {
		op := mixedOps[i%len(mixedOps)]
		mixedSrc.WriteString(op + "true")
		mixedWant.WriteString(" " + op + " true")
	}
	mixedSrc.WriteString("\n")
	mixedWant.WriteString("\n")

	caseSrc := "case x in (a) a" + strings.Repeat("|a", n-1) + ";; esac\n"
	multiLine := strings.Repeat("true &&\n", n-1) + "true\n"
	return []flatChain{
		commandChain("AndList", "&&", n),
		commandChain("OrList", "||", n),
		commandChain("Pipeline", "|", n),
		commandChain("PipelineStderr", "|&", n),
		{
			name: "Mixed",
			src:  mixedSrc.String(),
			want: mixedWant.String(),
			root: func(f *File) Node { return f.Stmts[0].Cmd },
			pos:  0,
			end:  uint(mixedSrc.Len() - 1),
		},
		{
			// A pipeline in a case item's statement list.
			name: "CaseItemPipeline",
			src:  caseSrc,
			// The printer drops the optional "(" and keeps a short case on one line.
			want: "case x in a) a" + strings.Repeat(" | a", n-1) + " ;; esac\n",
			root: func(f *File) Node {
				return f.Stmts[0].Cmd.(*CaseClause).Items[0].Stmts[0].Cmd
			},
			pos: uint(len("case x in (a) ")),
			end: uint(len(caseSrc) - len(";; esac\n")),
		},
		{
			// A list split over lines is printed with one indented continuation.
			name:      "MultiLineAndList",
			src:       multiLine,
			want:      "true" + strings.Repeat(" &&\n\ttrue", n-1) + "\n",
			root:      func(f *File) Node { return f.Stmts[0].Cmd },
			pos:       0,
			end:       uint(len(multiLine) - 1),
			multiLine: true,
		},
		{
			// BinaryNextLine moves each operator to the start of the next line.
			name:      "BinaryNextLineAndList",
			src:       multiLine,
			printer:   []PrinterOption{BinaryNextLine(true)},
			want:      "true" + strings.Repeat(" \\\n\t&& true", n-1) + "\n",
			root:      func(f *File) Node { return f.Stmts[0].Cmd },
			pos:       0,
			end:       uint(len(multiLine) - 1),
			multiLine: true,
		},
		arithmChain("ArithmAdd", "+", n),
		arithmChain("ArithmSub", "-", n),
		arithmChain("ArithmMul", "*", n),
		arithmChain("ArithmShl", "<<", n),
		arithmChain("ArithmAnd", "&&", n),
	}
}

// countChainNodes counts a chain's binary nodes and its elements (commands, or
// arithmetic literals) with Walk, so long chains also exercise traversal.
func countChainNodes(root Node) (binary, elements int) {
	_, arithm := root.(ArithmExpr)
	Walk(root, func(node Node) bool {
		switch node.(type) {
		case *BinaryCmd, *BinaryArithm:
			binary++
		case *CallExpr:
			elements++
		case *Lit:
			if arithm {
				elements++
			}
		}
		return true
	})
	return binary, elements
}

// Chain operators cost no nesting units, so a long flat chain parses under an
// embedding limit and under the default limit, and prints and positions
// correctly.
func TestForkFlatChainsParse(t *testing.T) {
	t.Parallel()
	for _, test := range flatChains(flatChainLinks) {
		for _, limit := range []struct {
			name  string
			units int // zero selects the default limit
		}{{"Limit128", 128}, {"Default", 0}} {
			t.Run(test.name+"/"+limit.name, func(t *testing.T) {
				t.Parallel()
				p := NewParser(KeepComments(true), ParseNestingLimit(limit.units))
				f, err := p.Parse(strings.NewReader(test.src), "")
				qt.Assert(t, qt.IsNil(err))

				var sb strings.Builder
				qt.Assert(t, qt.IsNil(NewPrinter(test.printer...).Print(&sb, f)))
				qt.Assert(t, qt.Equals(sb.String(), test.want))

				root := test.root(f)
				qt.Assert(t, qt.Equals(root.Pos().Offset(), test.pos))
				qt.Assert(t, qt.Equals(root.End().Offset(), test.end))
				// Columns beyond the column limit are not recorded, so lines and
				// offsets locate the ends of long one-line chains.
				qt.Assert(t, qt.Equals(root.Pos().Line(), uint(1)))
				qt.Assert(t, qt.Equals(root.Pos().Col(), test.pos+1))
				if test.multiLine {
					qt.Assert(t, qt.Equals(root.End().Line(), uint(flatChainLinks)))
				} else {
					qt.Assert(t, qt.Equals(root.End().Line(), uint(1)))
				}
				binary, elements := countChainNodes(root)
				qt.Assert(t, qt.Equals(binary, flatChainLinks-1))
				qt.Assert(t, qt.Equals(elements, flatChainLinks))
			})
		}
	}
}

// parseWithLimit128 parses src under an embedding limit of 128 units.
func parseWithLimit128(src string) error {
	_, err := NewParser(ParseNestingLimit(128)).Parse(strings.NewReader(src), "")
	return err
}

func assertDepthBoundary(t *testing.T, gen func(k int) string, last int) {
	t.Helper()
	qt.Assert(t, qt.IsNil(parseWithLimit128(gen(last))))
	var depthErr ParseDepthError
	qt.Assert(t, qt.ErrorAs(parseWithLimit128(gen(last+1)), &depthErr))
	qt.Assert(t, qt.Equals(depthErr, ParseDepthError{MaxDepth: 128, Depth: 129}))
}

func nestN(k int, open, mid, close string) string {
	return strings.Repeat(open, k) + mid + strings.Repeat(close, k)
}

// Nesting inside the last element of a long chain is counted from the chain's
// own depth, so it is refused at the same boundary as without the chain.
func TestForkFlatChainElementNesting(t *testing.T) {
	t.Parallel()
	cmdSubst := func(k int) string { return "echo " + nestN(k, "$(", ":", ")") + "\n" }
	arithParens := func(k int) string { return nestN(k, "(", "1", ")") }
	mixedPrefix := func(n int) string {
		ops := []string{" && ", " || ", " | ", " |& "}
		var sb strings.Builder
		sb.WriteString("true")
		for i := range n - 1 {
			sb.WriteString(ops[i%len(ops)] + "true")
		}
		return sb.String() + " && "
	}
	tests := []struct {
		name       string
		standalone func(k int) string
		chained    func(k int) string
		last       int // last accepted k for the standalone form
	}{
		{
			"AndList", cmdSubst,
			func(k int) string { return strings.Repeat("true && ", flatChainLinks) + cmdSubst(k) },
			63,
		},
		{
			"Pipeline", cmdSubst,
			func(k int) string { return strings.Repeat("true | ", flatChainLinks) + cmdSubst(k) },
			63,
		},
		{
			"Mixed", cmdSubst,
			func(k int) string { return mixedPrefix(flatChainLinks) + cmdSubst(k) },
			63,
		},
		{
			"Arithm",
			func(k int) string { return "echo $(( " + arithParens(k) + " ))\n" },
			func(k int) string {
				return "echo $(( " + strings.Repeat("1+", flatChainLinks) + arithParens(k) + " ))\n"
			},
			125,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertDepthBoundary(t, test.standalone, test.last)
			assertDepthBoundary(t, test.chained, test.last)
		})
	}
}

// Genuine recursion keeps its unit boundaries under a 128-unit limit: the last
// accepted repetition count k is followed by a 129/128 refusal at k+1.
func TestForkGenuineNestingBoundaries(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		gen  func(k int) string
		last int
	}{
		{"Subshell", func(k int) string { return nestN(k, "( ", ":", " )") + "\n" }, 127},
		{"CmdSubst", func(k int) string { return "echo " + nestN(k, "$(", ":", ")") + "\n" }, 63},
		{"ArithmParens", func(k int) string { return "echo $(( " + nestN(k, "(", "1", ")") + " ))\n" }, 125},
		{"ArithmPow", func(k int) string { return "echo $(( " + strings.Repeat("2**", k) + "2 ))\n" }, 125},
		{"ArithmAssign", func(k int) string { return "echo $(( " + strings.Repeat("a=", k) + "1 ))\n" }, 125},
		{"ArithmTernary", func(k int) string { return "echo $(( " + strings.Repeat("a?1:", k) + "1 ))\n" }, 125},
		{"ArithmUnaryMinus", func(k int) string { return "echo $(( " + strings.Repeat("- ", k) + "1 ))\n" }, 125},
		{"ArithmUnaryNot", func(k int) string { return "echo $(( " + strings.Repeat("!", k) + "1 ))\n" }, 125},
		{"If", func(k int) string { return nestN(k, "if :; then ", ":", "; fi") + "\n" }, 127},
		{"While", func(k int) string { return nestN(k, "while :; do ", ":", "; done") + "\n" }, 127},
		{"Block", func(k int) string { return nestN(k, "{ ", ":", "; }") + "\n" }, 127},
		{"Elif", func(k int) string { return nestN(k, "if :; then :\nel", "if :; then :\n", "") + "fi\n" }, 126},
		{"TestAndList", func(k int) string { return "[[ " + strings.Repeat("x && ", k) + "x ]]\n" }, 125},
		{"TestNot", func(k int) string { return "[[ " + strings.Repeat("! ", k) + "x ]]\n" }, 125},
		// Forms that once escaped an earlier lexical depth estimate.
		{"PatternCmdSubst", func(k int) string { return "echo ${x#" + nestN(k, "$(", "x", ")") + "}\n" }, 63},
		{"QuotedParamExp", func(k int) string { return "echo \"" + nestN(k, "${a:-", "x", "}") + "\"\n" }, 125},
		{"HeredocParamExp", func(k int) string { return "cat <<EOF\n" + nestN(k, "${a:-", "x", "}") + "\nEOF\n" }, 126},
		{"ArithmSubscript", func(k int) string { return "echo $((" + nestN(k, "a[", "1", "]") + "))\n" }, 125},
		{"SliceTernary", func(k int) string { return "echo ${x:" + strings.Repeat("a?1:", k) + "1}\n" }, 125},
		{"BracketAssign", func(k int) string { return "echo $[ " + strings.Repeat("a=", k) + "1 ]\n" }, 125},
		{"LetAssign", func(k int) string { return "let " + strings.Repeat("a=", k) + "1\n" }, 126},
		{"MultiLineAssign", func(k int) string { return "echo $((\n" + strings.Repeat("a=\n", k) + "1\n))\n" }, 125},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertDepthBoundary(t, test.gen, test.last)
		})
	}
}

// depthTracker records the deepest call stack seen by its record method. Its
// trace is large enough not to saturate for the chain lengths used below.
type depthTracker struct {
	pcs []uintptr
	max int
}

func newDepthTracker() *depthTracker { return &depthTracker{pcs: make([]uintptr, 1<<14)} }

func (d *depthTracker) record() { d.max = max(d.max, runtime.Callers(0, d.pcs)) }

// depthBuffer replaces the printer's internal buffered writer, recording the
// call depth of every write the printer makes. The output writer would not
// do: the tabwriter in front of it holds a single-line chain until the end.
type depthBuffer struct {
	*bufio.Writer
	depth *depthTracker
}

func (b depthBuffer) Write(p []byte) (int, error) {
	b.depth.record()
	return b.Writer.Write(p)
}

func (b depthBuffer) WriteString(s string) (int, error) {
	b.depth.record()
	return b.Writer.WriteString(s)
}

func (b depthBuffer) WriteByte(c byte) error {
	b.depth.record()
	return b.Writer.WriteByte(c)
}

// depthSlack absorbs call-path differences between a short and a long chain
// that do not depend on the chain's length.
const depthSlack = 8

// Short and long chains are compared by the deepest call that prints, walks,
// or positions them. Long words make the printer flush partway through both.
func TestForkFlatChainCallDepth(t *testing.T) {
	t.Parallel()
	word := strings.Repeat("x", 64)
	cmdChain := func(n int) string { return word + strings.Repeat(" && "+word, n-1) + "\n" }
	arithChain := func(n int) string { return "echo $((" + word + strings.Repeat("+"+word, n-1) + "))\n" }
	parse := func(t *testing.T, src string) *File {
		f, err := NewParser().Parse(strings.NewReader(src), "")
		qt.Assert(t, qt.IsNil(err))
		return f
	}
	const small, large = 100, 1_000
	tests := []struct {
		name  string
		depth func(t *testing.T, n int) int
	}{
		{"PrintCommandChain", func(t *testing.T, n int) int {
			return printDepth(t, parse(t, cmdChain(n)))
		}},
		{"PrintArithmChain", func(t *testing.T, n int) int {
			return printDepth(t, parse(t, arithChain(n)))
		}},
		{"WalkCommandChain", func(t *testing.T, n int) int {
			return walkDepth(parse(t, cmdChain(n)))
		}},
		{"WalkArithmChain", func(t *testing.T, n int) int {
			return walkDepth(parse(t, arithChain(n)))
		}},
		{"BinaryArithmPos", func(t *testing.T, n int) int {
			probe := &depthProbe{depth: newDepthTracker()}
			var x ArithmExpr = probe
			for range n - 1 {
				x = &BinaryArithm{Op: Add, X: x, Y: &Word{Parts: []WordPart{&Lit{Value: "1"}}}}
			}
			x.Pos()
			return probe.depth.max
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Short and long chains share one goroutine, so their depths compare.
			shallow, deep := test.depth(t, small), test.depth(t, large)
			qt.Assert(t, qt.IsTrue(deep <= shallow+depthSlack), qt.Commentf("depth %d for %d links, %d for %d", shallow, small, deep, large))
		})
	}
}

func printDepth(t *testing.T, f *File) int {
	depth := newDepthTracker()
	p := NewPrinter()
	p.w = depthBuffer{Writer: bufio.NewWriter(nil), depth: depth}
	qt.Assert(t, qt.IsNil(p.Print(io.Discard, f)))
	return depth.max
}

func walkDepth(f *File) int {
	depth := newDepthTracker()
	Walk(f, func(Node) bool {
		depth.record()
		return true
	})
	return depth.max
}

// depthProbe is an arithmetic leaf that records the call depth of Pos.
type depthProbe struct{ depth *depthTracker }

func (p *depthProbe) Pos() Pos      { p.depth.record(); return Pos{} }
func (p *depthProbe) End() Pos      { return Pos{} }
func (*depthProbe) arithmExprNode() {}
