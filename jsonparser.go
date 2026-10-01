package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// testEvent is one line of `go test -json` (the test2json format).
type testEvent struct {
	Action  string
	Package string
	Test    string
	Output  string
	Elapsed float64
}

// ParseAuto looks at the first non-blank byte of the input and hands
// it to ParseJSON if it is '{', otherwise to Parse. That lets the same
// pipe work for `go test -v` and `go test -json` without a flag.
func ParseAuto(r io.Reader) (*Report, error) {
	br := bufio.NewReader(r)
	head, err := br.Peek(512)
	if err != nil && err != io.EOF && err != bufio.ErrBufferFull {
		return nil, fmt.Errorf("reading test output: %w", err)
	}
	if t := bytes.TrimLeft(head, " \t\r\n"); len(t) > 0 && t[0] == '{' {
		return ParseJSON(br)
	}
	return Parse(br)
}

// ParseJSON reads `go test -json` output and builds a Report. Lines
// that are not valid events (go build noise, for instance) are kept
// as output on a package named "unknown" rather than aborting.
func ParseJSON(r io.Reader) (*Report, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1<<20)

	rep := &Report{}
	pkgs := map[string]*Package{}
	tests := map[string]*TestResult{}

	pkgFor := func(name string) *Package {
		if p, ok := pkgs[name]; ok {
			return p
		}
		p := &Package{Name: name}
		pkgs[name] = p
		rep.Packages = append(rep.Packages, p)
		return p
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var ev testEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil || ev.Action == "" {
			p := pkgFor("unknown")
			p.Output = append(p.Output, line)
			continue
		}

		pkg := pkgFor(ev.Package)
		duration := fmt.Sprintf("%.2fs", ev.Elapsed)

		if ev.Test == "" {
			switch ev.Action {
			case "output":
				if text := pkgOutputLine(ev.Output); text != "" {
					pkg.Output = append(pkg.Output, text)
				}
			case "pass", "fail":
				pkg.Status = Pass
				if ev.Action == "fail" {
					pkg.Status = Fail
				}
				pkg.Duration = duration
			case "skip":
				// go test reports "[no test files]" as a package-level
				// skip; a package whose tests all skipped still has
				// test events, so only the empty case is NoTests.
				pkg.Status = Pass
				if len(pkg.Tests) == 0 {
					pkg.NoTests = true
				} else {
					pkg.Duration = duration
				}
			}
			continue
		}

		key := ev.Package + "\x00" + ev.Test
		t, ok := tests[key]
		if !ok {
			t = &TestResult{Name: ev.Test, Status: -1}
			tests[key] = t
			pkg.Tests = append(pkg.Tests, t)
		}

		switch ev.Action {
		case "output":
			if text := testOutputLine(ev.Output); text != "" {
				t.Output = append(t.Output, text)
			}
		case "pass", "fail", "skip":
			t.Status = statusFromWord(strings.ToUpper(ev.Action))
			t.Duration = duration
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading test output: %w", err)
	}

	if len(rep.Packages) == 0 {
		rep.Packages = append(rep.Packages, &Package{Name: "unknown"})
	}
	return rep, nil
}

// testOutputLine drops the framing lines go test prints around a
// test (=== RUN, --- FAIL) since the events already carry that
// information, and returns the rest without its trailing newline.
func testOutputLine(s string) string {
	s = strings.TrimRight(s, "\r\n")
	trimmed := strings.TrimSpace(s)
	if strings.HasPrefix(trimmed, "=== ") || strings.HasPrefix(trimmed, "--- ") {
		return ""
	}
	return s
}

// pkgOutputLine keeps package-level output that is not the standard
// PASS/FAIL/ok trailer, which the text parser also ignores.
func pkgOutputLine(s string) string {
	s = strings.TrimRight(s, "\r\n")
	if strings.TrimSpace(s) == "" || s == "PASS" || s == "FAIL" || strings.HasPrefix(s, "exit status") {
		return ""
	}
	if pkgRe.MatchString(s) || noTestRe.MatchString(s) {
		return ""
	}
	return s
}
