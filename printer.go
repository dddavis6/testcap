package main

import (
	"fmt"
	"io"
	"strings"
)

// Print writes a human-readable summary of the report to w: one
// line per package with pass/fail/skip counts, then the full output
// of every failing test, then a totals line.
func Print(w io.Writer, r *Report) {
	nameWidth := len("PACKAGE")
	for _, pkg := range r.Packages {
		if len(pkg.Name) > nameWidth {
			nameWidth = len(pkg.Name)
		}
	}

	fmt.Fprintf(w, "%-*s  %5s %5s %5s  %s\n", nameWidth, "PACKAGE", "PASS", "FAIL", "SKIP", "TIME")

	var totalPass, totalFail, totalSkip, totalPkgs int
	var failedPkgs []*Package

	for _, pkg := range r.Packages {
		var pass, fail, skip int
		for _, t := range pkg.Tests {
			switch t.Status {
			case Pass:
				pass++
			case Fail:
				fail++
			case Skip:
				skip++
			}
		}
		totalPass += pass
		totalFail += fail
		totalSkip += skip
		totalPkgs++

		dur := pkg.Duration
		if pkg.NoTests {
			dur = "-"
		}
		fmt.Fprintf(w, "%-*s  %5d %5d %5d  %s\n", nameWidth, pkg.Name, pass, fail, skip, dur)

		if fail > 0 {
			failedPkgs = append(failedPkgs, pkg)
		}
	}

	if len(failedPkgs) > 0 {
		fmt.Fprintln(w, "\nFAILURES")
		for _, pkg := range failedPkgs {
			fmt.Fprintf(w, "\n  %s\n", pkg.Name)
			for _, t := range pkg.Tests {
				if t.Status != Fail {
					continue
				}
				fmt.Fprintf(w, "    %s (%s)\n", t.Name, t.Duration)
				for _, line := range t.Output {
					fmt.Fprintf(w, "      %s\n", strings.TrimSpace(line))
				}
			}
		}
	}

	fmt.Fprintf(w, "\n%d packages, %d passed, %d failed, %d skipped\n",
		totalPkgs, totalPass, totalFail, totalSkip)
}
