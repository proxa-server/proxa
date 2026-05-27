// Command coverage-gate parses a Go coverage profile (`coverage.out`)
// and prints a per-package coverage table, highlighting packages below
// a configurable threshold.
//
// In v0.4.2 the tool is reporting-only — exit code is always 0 when the
// input is valid. A future release (v0.4.3+) may flip the exit code to
// signal threshold violations, but that is gated behind separate spec
// work. See specs/006-test-foundation-public-images/contracts/coverage-gate.md.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

func main() {
	threshold := flag.Float64("threshold", 60.0, "percentage below which a package is highlighted")
	allowlistPath := flag.String("allowlist", ".coverage-allowlist", "file of import paths exempted from the threshold")
	format := flag.String("format", "text", "output format: text or json")
	flag.Parse()

	if *threshold < 0 || *threshold > 100 {
		fmt.Fprintf(os.Stderr, "error: -threshold %g out of range (expected 0-100)\n", *threshold)
		os.Exit(2)
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintf(os.Stderr, "error: -format %q must be text or json\n", *format)
		os.Exit(2)
	}
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: coverage-gate [flags] coverage.out")
		os.Exit(1)
	}

	pkgs, err := parseCoverage(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	allowlist, err := loadAllowlist(*allowlistPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: load allowlist: %v\n", err)
		os.Exit(1)
	}

	report := buildReport(pkgs, *threshold, allowlist)

	switch *format {
	case "json":
		_ = json.NewEncoder(os.Stdout).Encode(report)
	default:
		writeText(os.Stdout, report)
	}

	// v0.4.2: always exit 0 (reporting-only). v0.4.3+ may flip on threshold violation.
	os.Exit(0)
}

// pkgCoverage holds per-statement-block coverage counters for one package.
type pkgCoverage struct {
	covered, total int
}

// parseCoverage reads a Go coverage profile and aggregates per-package counts.
// File shape (one block per line):
//
//	mode: atomic|set|count
//	<pkg>/<file>:<startLine>.<startCol>,<endLine>.<endCol> <numStatements> <count>
//
// We aggregate by the package portion of the path (everything before the
// last `/`). A statement is "covered" when count > 0.
func parseCoverage(path string) (map[string]pkgCoverage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", path, err)
	}
	defer f.Close()

	out := map[string]pkgCoverage{}
	sc := bufio.NewScanner(f)
	first := true
	for sc.Scan() {
		line := sc.Text()
		if first {
			first = false
			if !strings.HasPrefix(line, "mode:") {
				return nil, errors.New("first line must be `mode: ...`")
			}
			continue
		}
		if line == "" {
			continue
		}
		// Format: path:range numStatements count
		// e.g. github.com/x/y/file.go:10.5,12.20 3 1
		colonIdx := strings.LastIndex(line, ":")
		if colonIdx < 0 {
			return nil, fmt.Errorf("malformed line (no colon): %q", line)
		}
		pkgFile := line[:colonIdx]
		// pkg is everything before the last '/'.
		slashIdx := strings.LastIndex(pkgFile, "/")
		if slashIdx < 0 {
			return nil, fmt.Errorf("malformed line (no package): %q", line)
		}
		pkg := pkgFile[:slashIdx]

		// rest = `range numStatements count`
		rest := line[colonIdx+1:]
		fields := strings.Fields(rest)
		if len(fields) != 3 {
			return nil, fmt.Errorf("malformed line (expected 3 fields after colon): %q", line)
		}
		numStmts, err := strconv.Atoi(fields[1])
		if err != nil {
			return nil, fmt.Errorf("bad statement count %q: %w", fields[1], err)
		}
		count, err := strconv.Atoi(fields[2])
		if err != nil {
			return nil, fmt.Errorf("bad coverage count %q: %w", fields[2], err)
		}
		c := out[pkg]
		c.total += numStmts
		if count > 0 {
			c.covered += numStmts
		}
		out[pkg] = c
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}
	if len(out) == 0 {
		return nil, errors.New("no coverage data found")
	}
	return out, nil
}

// loadAllowlist reads a newline-delimited file of import paths.
// Comments (lines starting with `#`) and blank lines are ignored.
// A missing file is NOT an error — returns an empty set.
func loadAllowlist(path string) (map[string]bool, error) {
	out := map[string]bool{}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return out, nil
		}
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out[line] = true
	}
	return out, sc.Err()
}

// reportEntry is one row in the output report.
type reportEntry struct {
	Path            string  `json:"path"`
	CoveragePercent float64 `json:"coverage_percent"`
	BelowThreshold  bool    `json:"below_threshold"`
	Allowlisted     bool    `json:"allowlisted"`
}

// report is the full payload (text + json modes share it).
type report struct {
	ThresholdPercent    float64       `json:"threshold_percent"`
	OverallPercent      float64       `json:"overall_percent"`
	PackageCount        int           `json:"package_count"`
	BelowThresholdCount int           `json:"below_threshold_count"`
	AllowlistedCount    int           `json:"allowlisted_count"`
	Packages            []reportEntry `json:"packages"`
}

func buildReport(pkgs map[string]pkgCoverage, threshold float64, allowlist map[string]bool) report {
	entries := make([]reportEntry, 0, len(pkgs))
	var totalCovered, totalAll int
	below := 0
	for path, c := range pkgs {
		percent := 0.0
		if c.total > 0 {
			percent = 100.0 * float64(c.covered) / float64(c.total)
		}
		isAllowlisted := allowlist[path]
		isBelow := percent < threshold && !isAllowlisted
		entries = append(entries, reportEntry{
			Path:            path,
			CoveragePercent: percent,
			BelowThreshold:  isBelow,
			Allowlisted:     isAllowlisted,
		})
		totalCovered += c.covered
		totalAll += c.total
		if isBelow {
			below++
		}
	}
	// Sort ascending by coverage; alphabetical secondary.
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].CoveragePercent != entries[j].CoveragePercent {
			return entries[i].CoveragePercent < entries[j].CoveragePercent
		}
		return entries[i].Path < entries[j].Path
	})
	overall := 0.0
	if totalAll > 0 {
		overall = 100.0 * float64(totalCovered) / float64(totalAll)
	}
	return report{
		ThresholdPercent:    threshold,
		OverallPercent:      overall,
		PackageCount:        len(entries),
		BelowThresholdCount: below,
		AllowlistedCount:    len(allowlist),
		Packages:            entries,
	}
}

func writeText(w io.Writer, r report) {
	fmt.Fprintf(w, "Coverage report (threshold: %.0f%%, %d packages, %d allowlisted)\n\n",
		r.ThresholdPercent, r.PackageCount, r.AllowlistedCount)
	fmt.Fprintln(w, "  Package                                                          Coverage")
	fmt.Fprintln(w, "  -------                                                          --------")
	for _, e := range r.Packages {
		prefix := " "
		coverStr := fmt.Sprintf("%5.1f%%", e.CoveragePercent)
		switch {
		case e.Allowlisted:
			prefix = "~"
			coverStr = "n/a (allowlisted)"
		case e.BelowThreshold:
			prefix = "⚠"
		}
		fmt.Fprintf(w, "%s %-64s %s\n", prefix, e.Path, coverStr)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Summary:")
	fmt.Fprintf(w, "  Overall coverage:   %.1f%%\n", r.OverallPercent)
	fmt.Fprintf(w, "  Packages reported:  %d\n", r.PackageCount)
	fmt.Fprintf(w, "  Below threshold:    %d\n", r.BelowThresholdCount)
	fmt.Fprintf(w, "  Allowlisted:        %d\n", r.AllowlistedCount)
	fmt.Fprintln(w, "  Exit:               0 (reporting-only in v0.4.2)")
}
