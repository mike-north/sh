// Comment advancement must preserve upstream trees, positions, and errors while
// consuming bounded call-stack space for a flat run of comments. The recorded
// compatibility hashes come from pristine v3.14.1, not this maintained parser.
package syntax_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
	"mvdan.cc/sh/v3/syntax/typedjson"
)

// commentReader exposes refill boundaries and optional terminal reader failure.
// It also observes caller depth without walking or otherwise inspecting the AST.
type commentReader struct {
	source       string
	chunk        int
	fault        bool
	maximumDepth int
}

func (r *commentReader) Read(p []byte) (int, error) {
	var callers [512]uintptr
	if depth := runtime.Callers(0, callers[:]); depth > r.maximumDepth {
		r.maximumDepth = depth
	}
	if len(r.source) == 0 {
		if r.fault {
			return 0, errors.New("synthetic reader fault")
		}
		return 0, io.EOF
	}
	if len(p) > r.chunk {
		p = p[:r.chunk]
	}
	n := copy(p, r.source)
	r.source = r.source[n:]
	if len(r.source) == 0 && r.fault {
		return n, errors.New("synthetic reader fault")
	}
	return n, nil
}

// A fixed-size trace can saturate on the old recursion. The desired property is
// that growing a flat run does not grow reader-call depth, not an absolute stack
// budget for all callers or syntactically nested programs.
func TestCommentRunReaderCallDepthDoesNotGrow(t *testing.T) {
	depth := func(comments int) int {
		r := &commentReader{source: strings.Repeat("# comment\n", comments), chunk: 16}
		if _, err := syntax.NewParser().Parse(r, "comments.sh"); err != nil {
			t.Fatal(err)
		}
		return r.maximumDepth
	}
	small, large := depth(16), depth(5_000)
	if large > small+4 {
		t.Fatalf("flat comments grew reader call depth from %d to %d", small, large)
	}
}

// Each fingerprint covers the entire typed tree (including every Pos/End and
// stored position field), plus the returned error type, text, and byte position.
type commentOracleCase struct {
	Name         string
	Source       string
	Variant      syntax.LangVariant
	KeepComments bool
	StopAt       string
	Fault        bool
	SHA256       string
	ChunkSHA256  map[string]string
}

func commentSnapshot(reader io.Reader, fixture commentOracleCase) ([]byte, error) {
	options := []syntax.ParserOption{syntax.Variant(fixture.Variant), syntax.KeepComments(fixture.KeepComments)}
	if fixture.StopAt != "" {
		options = append(options, syntax.StopAt(fixture.StopAt))
	}
	file, parseError := syntax.NewParser(options...).Parse(reader, "comments.sh")
	var tree bytes.Buffer
	if err := typedjson.Encode(&tree, file); err != nil {
		return nil, err
	}
	result := struct {
		File      json.RawMessage
		ErrorType string
		ErrorText string
		Offset    uint
		Line      uint
		Col       uint
	}{File: tree.Bytes()}
	if parseError != nil {
		result.ErrorType = fmt.Sprintf("%T", parseError)
		result.ErrorText = parseError.Error()
		if err, ok := parseError.(syntax.ParseError); ok {
			result.Offset, result.Line, result.Col = err.Pos.Offset(), err.Pos.Line(), err.Pos.Col()
		}
	}
	return json.Marshal(result)
}

func TestCommentIterationMatchesPristineUpstream(t *testing.T) {
	data, err := os.ReadFile("testdata/fork-comment-oracle.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []commentOracleCase
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			for _, chunk := range []int{1, 2, 3, 7, 1_024, 4_095, 4_096} {
				reader := &commentReader{source: fixture.Source, chunk: chunk, fault: fixture.Fault}
				data, err := commentSnapshot(reader, fixture)
				if err != nil {
					t.Fatal(err)
				}
				digest := sha256.Sum256(data)
				expected := fixture.SHA256
				if override, ok := fixture.ChunkSHA256[fmt.Sprint(chunk)]; ok {
					expected = override
				}
				if got := hex.EncodeToString(digest[:]); got != expected {
					t.Fatalf("chunk=%d: tree, positions, or error changed\ngot %s\n%s", chunk, got, data)
				}
			}
		})
	}
}
