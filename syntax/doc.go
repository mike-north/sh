// Copyright (c) 2016, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

// Package syntax implements parsing and formatting of shell programs.
// It supports POSIX Shell, Bash, mksh, Zsh, and Bats via [LangVariant].
//
// [NewParser] builds a syntax tree from source, [NewPrinter] formats it back
// into source and is the engine behind shfmt, and [Walk] and [Preorder]
// traverse it. [Simplify] removes redundant syntax, [Quote] quotes arbitrary
// strings as shell words, and [mvdan.cc/sh/v3/syntax/typedjson] encodes trees
// as JSON.
//
// The parser and printer decode Unicode scalar values in ANSI-C quoted
// heredoc delimiters as UTF-8 regardless of locale. Unsupported delimiters
// include ones with invalid UTF-8, embedded newlines, a leading tab after
// <<-, or bytes that would receive Bash's internal quote marker. In files
// using non-ASCII Unicode escapes in heredoc delimiters, the parser rejects
// any bare carriage return outside quotes or heredoc bodies. It also rejects
// a backslash followed by a carriage return outside a CR-preserving literal,
// even in the middle of a word.
// The printer derives a heredoc's closing line from its emitted opener.
// The printer reports an error for a source heredoc that uses a non-ASCII
// Unicode escape and has no matched closing line.
package syntax
