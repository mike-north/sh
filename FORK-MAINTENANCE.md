# Maintained syntax branch

This fork is the home for Go syntax corrections consumed by `sh-ast`. Its Go
module path remains `mvdan.cc/sh/v3`; consumers select an immutable fork revision
with a Go module replacement. The binding owns transport and its selected runtime
resource policy. This repository owns changes to Go grammar, syntax records,
parser accounting, traversal, and printing. It contains no TypeScript analysis or
ESLint integration.

The initial maintenance baseline is upstream **v3.14.1**, commit
`a3f0c75d21d918756fa38de8b5d3429efde7948b`. The `shast/v3.14.1` branch identifies
that baseline; correction PRs target it rather than the newer upstream default
branch. The upstream default branch remains available separately. Updating the
baseline is a reviewed operation, not an implicit part of moving these fixes.
The original BSD license and source copyright notices remain in place.

## Retained deltas

These changes were reassessed from sh-ast revision
`43390d3fc048e6a1c524d34579182c0010df99dc`. Existing tests were inputs to review,
not the reason to retain old output. The native regression tests assert specific
shell values, syntax relationships, or resource outcomes. The source changes
correspond to the seven named patches at that revision; no patch application is
needed when building this fork.

| Delta | Purpose and native regression | Upstream status and reassessment trigger |
| --- | --- | --- |
| Comment iteration | Flat comments must not grow reader-call stack depth. `TestCommentRunReaderCallDepthDoesNotGrow` compares short and long runs. `TestCommentIterationMatchesPristineUpstream` checks complete recorded tree/position/error observations at seven reader chunk sizes. | Owned correction, absent from v3.14.1. The 140 fixture records and chunk-specific hashes were produced from isolated pristine v3.14.1. Reassess when upstream removes recursive comment advancement; retain the independent observations while checking the new baseline. |
| Default nesting refusal | Limit native statement, word, and expression work. `TestParseNestingLimit` is adapted from the upstream regression and covers the default 10,000-unit refusal and ordinary 1,000-unit acceptance. | Backport of official [0e0eb36](https://github.com/mvdan/sh/commit/0e0eb36bb6dd6a143842ffda25738e9a82dc915e), after the tag. On a baseline containing it, stop carrying a separate production delta and reconcile duplicate tests, keeping the flat chain delta below. |
| Flat chains | A unit counts parser recursion, not tree height. The operators of flat `&&` / `\|\|` lists, `\|` / `\|&` pipelines, and left-associative arithmetic chains cost no units, so each element is counted from the chain's own depth; upstream 0e0eb36 charges one unit per operator. The printer, `Walk`, `startsWithLparen`, and `(*BinaryArithm).Pos` follow the resulting left spines with loops. `TestForkFlatChainsParse` checks 10,000-link chains under a 128-unit and the default limit, with handwritten printed forms, root positions, and node counts. `TestForkFlatChainElementNesting` and `TestForkGenuineNestingBoundaries` keep nested and genuinely recursive boundaries; `TestForkFlatChainCallDepth` compares call depth for short and long chains. | Owned correction, absent from v3.14.1 and from upstream's accounting; reproduction [sh-ast #105](https://github.com/mike-north/sh-ast/issues/105). Reassess when upstream stops charging chain operators or makes these traversals iterative. Execution in `interp` and `expand`, `Simplify` on array indexes, and `typedjson` still recurse once per link. |
| Configurable resource mechanism | `ParseNestingLimit` selects a positive work limit or zero for the upstream default. `ParseDepthError` is a terminal resource refusal, not invalid grammar; whole-file Parse returns no partial File. `TestForkBudgetRefusalRecoveryAndReuse`, `TestForkBudgetBoundaryAndGrammarPrecedence`, and `TestForkBudgetRejectsNegativeOption` cover reset, recovery, exact boundaries, precedence, and configuration. | Owned embedding mechanism, absent from v3.14.1. The Go library does **not** select 128 by default; that number is an embedding test policy and sh-ast's separate choice. Reassess when upstream supplies equivalent configurable accounting/refusal semantics or an embedding no longer needs it. |
| Pending heredoc scope | A subshell opener must not hide pending outer heredocs. `TestForkPendingHeredocSubshell` checks attachment, later statements, multibyte data, `(` / `( :`, block/conditional controls, and the distinct command-substitution scope. | Owned correction, absent from v3.14.1; original reproduction [sh-ast #72](https://github.com/mike-north/sh-ast/issues/72). Remove the separate delta only when the selected upstream implementation passes these scope tests. |
| Literal VT/FF printing | Literal control bytes belong to shell data, comments, and delimiter words, not tabwriter layout. `TestForkPrinterControlBytes` checks handwritten output and Bash values; written ANSI-C escapes and horizontal tabs are controls. | Owned correction, absent from v3.14.1; [sh-ast #74](https://github.com/mike-north/sh-ast/issues/74). Later upstream [0d47c1c](https://github.com/mvdan/sh/commit/0d47c1c93a50bd89cf4b0fdd56c8440b5684dde7) overlaps delimiter handling but is not wholesale equivalence. Reassess literals, comments, and delimiters separately when incorporating it. |
| Heredoc continuation tabs | Tabs following an active unquoted `<<-` continuation are value bytes. `TestForkHeredocContinuation` checks exact spelling, Bash stdout, chains, parity, tab-only lines, quoted delimiters, and noncontinuation controls, under default and KeepPadding. | Owned correction, absent from v3.14.1; [sh-ast #81](https://github.com/mike-north/sh-ast/issues/81). Remove only after the new upstream writer preserves the same independently stated logical-line values. |
| Zsh prefix order | Print option overrides before length/existence operators so reparsing retains state and `${=#a}` retains the value three. `TestForkZshPrefixOrder` checks native fields, both print modes, and installed Zsh. | Owned correction, absent from v3.14.1. Reassess against upstream parser/printer ordering at each baseline update; do not replace it merely because unrelated Zsh grammar changed. |

## Compatibility holds intentionally omitted

The former `printer-backslash-compat` patch is not carried. The selected printer
therefore has v3.14.1's trailing-backslash doubling, including caller-constructed
adjacent literals. Old TypeScript expectations do not justify preserving a
different Go printer. This is not a universal shell-correctness claim: the
handwritten source `printf '<%s>\n' ` followed by one backslash at EOF produces
`<>` plus newline in the observed Bash, whereas upstream's printed form produces
a backslash argument. That discrepancy is a bounded future correctness question,
not a compatibility reason to revert the shared printer.

The former `zsh-subscript-compat` rollback is not carried. V3.14.1 reads reverse
subscript patterns as raw text. This accepts `foo*` without treating a pattern as
arithmetic, but Walk cannot expose an embedded `$(printf foo)` or parameter
expansion hidden inside that raw Lit. `TestForkSelectedUpstreamLimits` records the
selected representation and traversal limit; it does not prove there are no
commands in the source. Analysis consumers must retain uncertainty. Broader Zsh
pattern parsing is separate work requiring bounded grammar/quote contracts and
independent syntax evidence, not restoration of the earlier arithmetic model.
The character-class example `foo[0-9]` is also not solved by this migration:
pristine v3.14.1 refuses it, while the removed rollback printed it as `foo[0 - 9]`
and could change the selected array element. See [sh-ast #90](https://github.com/mike-north/sh-ast/issues/90).

## Validation record

Initial validation used Go 1.26.3 on darwin/arm64, Bash 5.3.9, and Zsh 5.9.
Tests were written and run against the pristine tag before applying production
corrections. The initial red run reproduced comment stack growth, missing nesting
refusals, pending-document failures, changed VT/FF bytes, continuation tab changes,
and reversed Zsh prefixes. Budget tests first failed for the absent native API;
after implementation, disabling actual limit selection made refusal and boundary
tests fail again. The mutation was restored before the final green run.

`go test -p 2 -parallel 4 ./syntax/... ./fileutil -count=1` passes. The complete
`go test -p 2 -parallel 4 ./... -count=1` passes every package except four
`interp/TestRunnerRunConfirm` Bash comparisons: `#374`, `#545`, `#1222`, and
`#1224`. The exact same four cases fail in a detached pristine v3.14.1 worktree
under the same host environment. They concern Bash tilde/HOME expansion and an
error after closing stderr; no interpreter code is changed here. This is a
recorded baseline limitation, not a claim that the full suite passes.

## Upstream upkeep

Before selecting a new baseline, fetch the `upstream` remote and inspect changes
since the recorded tag. Check each retained delta against the candidate source
and its independent regression outcomes; identify absorbed fixes and overlapping
changes. Update the delta table and baseline identity in the same review. Do not
auto-adopt new grammar while rebasing a runtime correction. Keep a recorded
selected revision for sh-ast's schema and WASM builds, and return to an upstream
dependency when it contains all required behavior. No update schedule or recurring
automation is selected by this record.
