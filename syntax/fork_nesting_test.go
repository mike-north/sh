// Default nesting refusal follows upstream commit 0e0eb36bb6dd6a143842ffda25738e9a82dc915e.
package syntax

import (
	"github.com/go-quicktest/qt"
	"runtime"
	"strings"
	"testing"
)

func TestParseNestingLimit(t *testing.T) {
	t.Parallel()

	nest := func(n int, open, mid, close string) string {
		return strings.Repeat(open, n) + mid + strings.Repeat(close, n)
	}
	tests := []struct {
		name string
		in   func(n int) string
	}{
		{"Subshell", func(n int) string { return nest(n, "(\n", ":", ")\n") }},
		{"FuncBody", func(n int) string { return nest(n, "f() ", "{ :; }", "") }},
		{"CmdSubst", func(n int) string { return nest(n, "$(", ":", ")") }},
		{"ParamExp", func(n int) string { return nest(n, "${a:-", "x", "}") }},
		{"AndChain", func(n int) string { return nest(n, ":&&\n", ":", "") }},
		{"Pipeline", func(n int) string { return nest(n, ":|\n", ":", "") }},
		{"ElifChain", func(n int) string { return nest(n, "if :; then :\nel", "if :; then :\n", "") + "fi" }},
		{"ArithmParens", func(n int) string { return "((" + nest(n, "(", "1", ")") + "))" }},
		{"ArithmAssign", func(n int) string { return "((" + nest(n, "a=", "1", "") + "))" }},
		{"ArithmChain", func(n int) string { return "((" + nest(n, "1+", "1", "") + "))" }},
		{"TestNot", func(n int) string { return "[[ " + nest(n, "! ", "x", "") + " ]]" }},
		{"TestAndChain", func(n int) string { return "[[ " + nest(n, "x && ", "x", "") + " ]]" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if runtime.GOOS == "js" && strings.HasPrefix(test.name, "Arithm") {
				// Each Go call is a wasm call, and arithmetic expressions
				// recurse through many functions per level.
				t.Skip("deep arithmetic recursion overflows the V8 stack")
			}
			t.Parallel()

			p := NewParser()
			_, err := p.Parse(strings.NewReader(test.in(1000)), "")
			qt.Assert(t, qt.IsNil(err))
			_, err = p.Parse(strings.NewReader(test.in(10_001)), "")
			qt.Assert(t, qt.ErrorMatches(err, `.*: nesting is deeper than 10000 levels`))
		})
	}
}
