// Fork corrections preserve independently specified shell values and syntax
// relationships. Only these handwritten programs are executed by the tests.
package syntax_test

import (
	"bytes"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// forkParse keeps parsing failures attached to their actual input fixture.
func forkParse(t *testing.T, source string, options ...syntax.ParserOption) *syntax.File {
	t.Helper()
	file, err := syntax.NewParser(options...).Parse(strings.NewReader(source), "fixture.sh")
	if err != nil {
		t.Fatalf("parse %q: %v", source, err)
	}
	return file
}

// forkPrint observes the public native printer, without using it as its own oracle.
func forkPrint(t *testing.T, file *syntax.File, options ...syntax.PrinterOption) string {
	t.Helper()
	var output bytes.Buffer
	if err := syntax.NewPrinter(options...).Print(&output, file); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

// forkStdout compares only hand-authored fixtures with their independently stated
// shell values. Missing optional dialect executables skip shell checks, not AST checks.
func forkStdout(t *testing.T, executable, source string) string {
	t.Helper()
	path, err := exec.LookPath(executable)
	if err != nil {
		t.Skipf("%s not installed", executable)
	}
	args := []string{"-c", source}
	if executable == "zsh" {
		args = []string{"-f", "-c", source}
	}
	cmd := exec.Command(path, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil || stderr.Len() != 0 {
		t.Fatalf("%s fixture: %v, stderr=%s", executable, err, &stderr)
	}
	return string(output)
}

// A subshell shares pending documents; command substitution has a separate scope.
func TestForkPendingHeredocSubshell(t *testing.T) {
	for _, opener := range []string{"(", "( :", "{", "if true; then"} {
		t.Run(opener, func(t *testing.T) {
			closer := ")"
			if opener == "{" {
				closer = "}"
			}
			if opener == "if true; then" {
				closer = "fi"
			}
			source := "cat <<X; " + opener + "\né🎉\nX\necho in\n" + closer + "\n"
			file := forkParse(t, source)
			if len(file.Stmts) != 2 || len(file.Stmts[0].Redirs) != 1 {
				t.Fatal("document moved or later statement lost")
			}
			doc := file.Stmts[0].Redirs[0].Hdoc
			if doc == nil || doc.Lit() != "é🎉\n" {
				t.Fatalf("document=%#v", doc)
			}
			if got := forkStdout(t, "bash", source); got != "é🎉\nin\n" {
				t.Fatalf("stdout=%q", got)
			}
		})
	}
	outer := "cat <<X; y=$(\necho in\n)\nbody\nX\n"
	file := forkParse(t, outer)
	if file.Stmts[0].Redirs[0].Hdoc.Lit() != "body\n" {
		t.Fatal("outer document scope changed")
	}
	_, err := syntax.NewParser().Parse(strings.NewReader("cat <<X; y=$(\nbody\nX\necho in\n)\n"), "")
	if err == nil || !strings.Contains(err.Error(), "unclosed here-document") {
		t.Fatalf("lost outer refusal: %v", err)
	}
}

// VT and FF are literal shell data, including in comments and document markers.
func TestForkPrinterControlBytes(t *testing.T) {
	for _, control := range []string{"\v", "\f"} {
		for _, source := range []string{
			"printf '%s' 'é" + control + "🎉'\n",
			"printf '%s' \"é" + control + "🎉\"\n",
			"printf '%s' é" + control + "🎉\n",
			"cat <<EOF\né" + control + "🎉\nEOF\n",
			"cat <<-EOF\n\té" + control + "🎉\nEOF\n",
			"cat <<E" + control + "OF\nbody\nE" + control + "OF\nprintf kept\n",
			"# é" + control + "🎉\nprintf kept\n",
		} {
			t.Run(source, func(t *testing.T) {
				printed := forkPrint(t, forkParse(t, source, syntax.KeepComments(true)))
				if printed != source {
					t.Fatalf("printed=%q, want=%q", printed, source)
				}
				if got, want := forkStdout(t, "bash", printed), forkStdout(t, "bash", source); got != want {
					t.Fatalf("stdout=%q, want=%q", got, want)
				}
			})
		}
	}
	// Written ANSI-C escapes and ordinary horizontal tabs keep their spelling.
	for _, source := range []string{"printf '%s' $'a\\vb'\n", "printf '%s' $'a\\fb'\n", "printf '%s' 'a\tb'\n"} {
		if got := forkPrint(t, forkParse(t, source)); got != source {
			t.Fatalf("control spelling=%q", got)
		}
	}
}

// A <<- continuation joins physical lines before stripping indentation. Leading
// tabs on the following physical line are value bytes rather than layout.
func TestForkHeredocContinuation(t *testing.T) {
	for _, fixture := range []struct{ body, printed, stdout string }{
		{"\tXa\\\n\tmore\n", "\t\tXa\\\n\tmore\n", "Xa\tmore\n"},
		{"\tXa\\\n\tmore\\\n\tend\n\tlast\n", "\t\tXa\\\n\tmore\\\n\tend\n\t\tlast\n", "Xa\tmore\tend\nlast\n"},
		{"\tXa\\\n\t\n\tlast\n", "\t\tXa\\\n\t\n\t\tlast\n", "Xa\t\nlast\n"},
		{"\tXa\\\\\n\tmore\n", "\t\tXa\\\\\n\t\tmore\n", "Xa\\\nmore\n"},
		{"\tXa\\\\\\\n\tmore\n", "\t\tXa\\\\\\\n\tmore\n", "Xa\\\tmore\n"},
		{"\tXa\n\tmore\n", "\t\tXa\n\t\tmore\n", "Xa\nmore\n"},
	} {
		for _, padding := range []bool{false, true} {
			source := "if true; then\ncat <<-EOF\n" + fixture.body + "\tEOF\nfi\n"
			expected := "if true; then\n\tcat <<-EOF\n" + fixture.printed + "\tEOF\nfi\n"
			printed := forkPrint(t, forkParse(t, source), syntax.KeepPadding(padding))
			if printed != expected {
				t.Fatalf("printed=%q, want=%q", printed, expected)
			}
			for _, program := range []string{source, printed} {
				if got := forkStdout(t, "bash", program); got != fixture.stdout {
					t.Fatalf("stdout=%q, want=%q", got, fixture.stdout)
				}
			}
		}
	}
	// Quoted document delimiters disable physical-line joining entirely.
	for _, delimiter := range []string{"'EOF'", "\"EOF\"", "E\\OF", "E'OF'"} {
		source := "if true; then\ncat <<-" + delimiter + "\n\tXa\\\n\tmore\n\tEOF\nfi\n"
		printed := forkPrint(t, forkParse(t, source))
		if got := forkStdout(t, "bash", printed); got != "Xa\\\nmore\n" {
			t.Fatalf("quoted stdout=%q", got)
		}
	}
}

// Zsh option overrides precede the length/existence operator. Reprinting must
// preserve both the parameter state and the independently known shell value.
func TestForkZshPrefixOrder(t *testing.T) {
	for _, padding := range []bool{false, true} {
		source := "a='a b'; printf '%s\\n' ${=#a}\n"
		file := forkParse(t, source, syntax.Variant(syntax.LangZsh))
		printed := forkPrint(t, file, syntax.KeepPadding(padding))
		if !strings.Contains(printed, "${=#a}") {
			t.Fatalf("prefix order=%q", printed)
		}
		reparsed := forkParse(t, printed, syntax.Variant(syntax.LangZsh))
		states := func(f *syntax.File) []string {
			var got []string
			syntax.Walk(f, func(n syntax.Node) bool {
				if p, ok := n.(*syntax.ParamExp); ok {
					got = append(got, p.Param.Value+":"+string(rune(p.Split))+":"+string(rune(p.GlobSubst))+":"+string(rune(p.RcExpand)))
					if !p.Length {
						t.Error("length lost")
					}
				}
				return true
			})
			return got
		}
		if !reflect.DeepEqual(states(file), states(reparsed)) {
			t.Fatal("parameter state changed")
		}
		for _, program := range []string{source, printed} {
			if got := forkStdout(t, "zsh", program); got != "3\n" {
				t.Fatalf("Zsh stdout=%q", got)
			}
		}
	}
}

// Compatibility is not a fork requirement. The selected upstream representation
// retains raw pattern text and cannot expose commands embedded in that text.
func TestForkSelectedUpstreamLimits(t *testing.T) {
	file := forkParse(t, "echo ${a[(r)foo*]}\n", syntax.Variant(syntax.LangZsh))
	if got := forkPrint(t, file); got != "echo ${a[(r)foo*]}\n" {
		t.Fatalf("pattern=%q", got)
	}
	file = forkParse(t, "echo ${a[(r)$(printf foo)]}\n", syntax.Variant(syntax.LangZsh))
	var commands []string
	syntax.Walk(file, func(n syntax.Node) bool {
		if c, ok := n.(*syntax.CallExpr); ok && len(c.Args) > 0 {
			commands = append(commands, c.Args[0].Lit())
		}
		return true
	})
	if !reflect.DeepEqual(commands, []string{"echo"}) {
		t.Fatalf("selected traversal=%v", commands)
	}
	// This records upstream spelling, not a claim of universal shell equivalence.
	file = forkParse(t, "echo \\")
	if got := forkPrint(t, file); got != "echo \\\\\n" {
		t.Fatalf("EOF backslash=%q", got)
	}
}
