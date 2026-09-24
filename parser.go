package main

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
)

type Status int

const (
	Pass Status = iota
	Fail
	Skip
)

func (s Status) String() string {
	switch s {
	case Pass:
		return "PASS"
	case Fail:
		return "FAIL"
	case Skip:
		return "SKIP"
	default:
		return "?"
	}
}

// TestResult is one leaf or subtest reported by `go test -v`.
type TestResult struct {
	Name     string
	Status   Status
	Duration string
	Output   []string
}

// Package groups the tests that ran together and the final
// pass/fail line go test prints for that import path.
type Package struct {
	Name     string
	Status   Status
	Duration string
	NoTests  bool
	Tests    []*TestResult
	Output   []string // build errors, panics, anything not tied to a test
}

type Report struct {
	Packages []*Package
}

var (
	runRe    = regexp.MustCompile(`^=== RUN\s+(\S+)$`)
	resultRe = regexp.MustCompile(`^(\s*)--- (PASS|FAIL|SKIP): (\S+) \(([\d.]+)s\)$`)
	pkgRe    = regexp.MustCompile(`^(ok|FAIL)\s+(\S+)\s+([\d.]+)s`)
	noTestRe = regexp.MustCompile(`^(ok|\?)\s+(\S+)\s+\[no test files\]$`)
)

// Parse reads `go test -v` style text output and builds a Report.
// It does not fail on unrecognized lines; those are attached as
// output to whichever test is currently running, or to the package
// if no test is running.
func Parse(r io.Reader) (*Report, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1<<20)

	rep := &Report{}
	current := &Package{Name: "unknown"}
	rep.Packages = append(rep.Packages, current)

	running := map[string]*TestResult{}
	var last *TestResult
	sawAnyPkgLine := false

	for scanner.Scan() {
		line := scanner.Text()

		if m := runRe.FindStringSubmatch(line); m != nil {
			t := &TestResult{Name: m[1], Status: -1}
			current.Tests = append(current.Tests, t)
			running[m[1]] = t
			last = t
			continue
		}

		if m := resultRe.FindStringSubmatch(line); m != nil {
			name := m[3]
			t, ok := running[name]
			if !ok {
				// Result with no matching RUN: record it anyway so
				// validation can flag it, but keep parsing.
				t = &TestResult{Name: name, Status: -1}
				current.Tests = append(current.Tests, t)
			}
			t.Status = statusFromWord(m[2])
			t.Duration = m[4] + "s"
			delete(running, name)
			last = t
			continue
		}

		if m := noTestRe.FindStringSubmatch(line); m != nil {
			current.Name = m[2]
			current.NoTests = true
			current.Status = Pass
			sawAnyPkgLine = true
			current = &Package{Name: "unknown"}
			rep.Packages = append(rep.Packages, current)
			running = map[string]*TestResult{}
			last = nil
			continue
		}

		if m := pkgRe.FindStringSubmatch(line); m != nil {
			current.Name = m[2]
			current.Duration = m[3] + "s"
			if m[1] == "ok" {
				current.Status = Pass
			} else {
				current.Status = Fail
			}
			sawAnyPkgLine = true
			current = &Package{Name: "unknown"}
			rep.Packages = append(rep.Packages, current)
			running = map[string]*TestResult{}
			last = nil
			continue
		}

		if line == "PASS" || line == "FAIL" || strings.HasPrefix(line, "exit status") {
			continue
		}

		if strings.TrimSpace(line) == "" {
			continue
		}

		if last != nil {
			last.Output = append(last.Output, line)
		} else {
			current.Output = append(current.Output, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading test output: %w", err)
	}

	// Drop the trailing empty package created after the last summary
	// line, and the leading placeholder if nothing was ever attached
	// to it.
	rep.Packages = trimEmptyPackages(rep.Packages, sawAnyPkgLine)

	return rep, nil
}

func trimEmptyPackages(pkgs []*Package, sawAnyPkgLine bool) []*Package {
	out := pkgs[:0:0]
	for _, p := range pkgs {
		if p.Name == "unknown" && len(p.Tests) == 0 && len(p.Output) == 0 {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 && !sawAnyPkgLine {
		return pkgs[:1]
	}
	return out
}

func statusFromWord(w string) Status {
	switch w {
	case "PASS":
		return Pass
	case "FAIL":
		return Fail
	case "SKIP":
		return Skip
	default:
		return -1
	}
}

// Validate checks the report for internal inconsistencies: tests
// that started but never finished, results with no matching RUN
// line, and a package status that disagrees with its own tests.
// It returns every problem found, not just the first.
func (r *Report) Validate() []error {
	var errs []error
	for _, pkg := range r.Packages {
		hasFail := false
		for _, t := range pkg.Tests {
			switch {
			case t.Status == -1 && t.Duration == "":
				errs = append(errs, fmt.Errorf("%s: %s: started with RUN but no PASS/FAIL/SKIP result", pkg.Name, t.Name))
			case t.Status == -1:
				errs = append(errs, fmt.Errorf("%s: %s: result reported but no matching RUN line", pkg.Name, t.Name))
			case t.Status == Fail:
				hasFail = true
			}
		}
		if pkg.NoTests {
			continue
		}
		if pkg.Duration == "" {
			errs = append(errs, fmt.Errorf("%s: no package-level ok/FAIL summary line found", pkg.Name))
			continue
		}
		if hasFail && pkg.Status != Fail {
			errs = append(errs, fmt.Errorf("%s: contains failing tests but package summary says %s", pkg.Name, pkg.Status))
		}
		if !hasFail && pkg.Status == Fail && len(pkg.Tests) > 0 {
			errs = append(errs, fmt.Errorf("%s: package summary says FAIL but no test in it failed", pkg.Name))
		}
	}
	return errs
}
