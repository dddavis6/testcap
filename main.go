// Command testcap parses `go test -v` output and prints a compact
// pass/fail summary, flagging output that doesn't add up (tests
// that never finished, package summaries that disagree with their
// own tests).
package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	failed, err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "testcap:", err)
		os.Exit(1)
	}
	if failed {
		os.Exit(1)
	}
}

// run does the actual work and reports whether any package in the
// input failed, so main can set the exit code without run itself
// reaching for os.Exit.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) (failed bool, err error) {
	var in io.Reader = stdin
	if len(args) > 0 {
		f, err := os.Open(args[0])
		if err != nil {
			return false, err
		}
		defer f.Close()
		in = f
	}

	report, err := ParseAuto(in)
	if err != nil {
		return false, err
	}

	if errs := report.Validate(); len(errs) > 0 {
		fmt.Fprintln(stderr, "testcap: input looks inconsistent:")
		for _, e := range errs {
			fmt.Fprintln(stderr, " -", e)
		}
		fmt.Fprintln(stderr)
	}

	Print(stdout, report)

	for _, pkg := range report.Packages {
		if pkg.Status == Fail {
			failed = true
		}
	}
	return failed, nil
}
