# testcap

`go test -v` output is meant for a human watching CI scroll by, not
for scripts. It has no schema: a failing subtest, a package that
never printed its `ok`/`FAIL` line because the build died, a test
that panicked mid-run and left no result at all - all of that is
just text, and most tools that "summarize" it quietly assume the
output is well-formed.

testcap parses that text into a small structure, checks that the
structure actually makes sense (every `RUN` got a result, every
package's summary line agrees with its own tests), and prints a
short report. If something doesn't add up it says so on stderr
instead of silently producing a summary that looks fine.

## Usage

Pipe `go test -v` straight into it:

```
go test -v ./... | testcap
```

or point it at a saved log:

```
testcap ci-output.txt
```

Example output:

```
PACKAGE                   PASS  FAIL  SKIP  TIME
example.com/pkg/parser       8     1     0  0.12s
example.com/pkg/printer      3     0     0  0.04s

FAILURES

  example.com/pkg/parser
    TestParse/empty_input (0.00s)
      parser_test.go:41: expected io.EOF, got nil

2 packages, 11 passed, 1 failed, 0 skipped
```

testcap exits 1 if any package failed, so it can sit in a CI
pipeline the same way `go test` itself does.

If the input is inconsistent - say a test started with `=== RUN`
but the log was truncated before its result - testcap prints a
warning to stderr and still summarizes what it could parse:

```
testcap: input looks inconsistent:
 - example.com/pkg/parser: TestParse/slow_case: started with RUN but no PASS/FAIL/SKIP result
```

## Install

```
go install .
```

Requires Go 1.22 or later. No third-party dependencies.

## Status

Handles the plain-text output of `go test -v` and the event stream
from `go test -json`. The format is detected from the first
character of the input, so no flag is needed:

```
go test -json ./... | testcap
```

Table-driven output with custom `t.Log` formatting is not covered
yet.
