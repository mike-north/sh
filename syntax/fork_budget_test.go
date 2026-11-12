// Native accounting is configurable so embeddings can refuse excessive work
// without describing otherwise valid shell input as a grammar error.
package syntax_test

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// A selected policy survives ordinary reset and remains terminal under recovery;
// no partial File may escape after the resource refusal.
func TestForkBudgetRefusalRecoveryAndReuse(t *testing.T) {
	source := "echo $(( " + strings.Repeat("a[", 200) + "1" + strings.Repeat("]", 200) + " ))"
	for _, recovery := range []int{0, 1, 100_000} {
		parser := syntax.NewParser(syntax.RecoverErrors(recovery), syntax.ParseNestingLimit(128))
		for range 20 {
			file, err := parser.Parse(strings.NewReader(source), "budget.sh")
			failure, ok := err.(syntax.ParseDepthError)
			if !ok || file != nil || failure.MaxDepth != 128 || failure.Depth != 129 {
				t.Fatalf("expected terminal 129/128 refusal without File: file=%v error=%v", file, err)
			}
			file, err = parser.Parse(strings.NewReader("echo é🎉"), "next.sh")
			if err != nil || file == nil || len(file.Stmts) != 1 {
				t.Fatalf("same-parser reuse failed: %v", err)
			}
		}
	}
}

// Work counts parser recursion; flat operator chains are free. Zero selects
// the upstream default; syntax errors encountered first retain their precedence.
func TestForkBudgetBoundaryAndGrammarPrecedence(t *testing.T) {
	for _, parens := range []int{125, 126} {
		source := "echo $(( " + strings.Repeat("(", parens) + "1" + strings.Repeat(")", parens) + " ))"
		_, err := syntax.NewParser(syntax.ParseNestingLimit(128)).Parse(strings.NewReader(source), "")
		if parens == 125 && err != nil {
			t.Fatalf("128 units refused: %v", err)
		}
		if parens == 126 {
			if _, ok := err.(syntax.ParseDepthError); !ok {
				t.Fatalf("129 units not refused: %v", err)
			}
		}
		if _, err := syntax.NewParser(syntax.ParseNestingLimit(0)).Parse(strings.NewReader(source), ""); err != nil {
			t.Fatalf("zero policy changed upstream acceptance: %v", err)
		}
	}
	_, err := syntax.NewParser(syntax.ParseNestingLimit(128)).Parse(strings.NewReader("; "+strings.Repeat("(", 200)), "")
	if _, ok := err.(syntax.ParseError); !ok {
		t.Fatalf("earlier syntax error lost precedence: %v", err)
	}
}

// Parser options reject invalid configuration independently from source syntax.
func TestForkBudgetRejectsNegativeOption(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("negative policy did not panic")
		}
	}()
	syntax.ParseNestingLimit(-1)
}
