package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseCoverageOut_HappyPath(t *testing.T) {
	pkgs, err := parseCoverage("testdata/happy.out")
	if err != nil {
		t.Fatalf("parseCoverage: %v", err)
	}
	// Three packages expected from the fixture.
	want := map[string]pkgCoverage{
		"github.com/proxa-server/proxa/internal/datadir": {covered: 3, total: 3},
		"github.com/proxa-server/proxa/internal/server":  {covered: 3, total: 10},
		"github.com/proxa-server/proxa/internal/web":     {covered: 0, total: 4},
	}
	if len(pkgs) != len(want) {
		t.Errorf("got %d packages, want %d", len(pkgs), len(want))
	}
	for k, w := range want {
		got, ok := pkgs[k]
		if !ok {
			t.Errorf("missing package %q", k)
			continue
		}
		if got != w {
			t.Errorf("pkg %q: got %+v, want %+v", k, got, w)
		}
	}
}

func TestParseCoverageOut_Empty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.out")
	if err := os.WriteFile(path, []byte("mode: atomic\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := parseCoverage(path)
	if err == nil {
		t.Errorf("expected error for empty coverage data")
	}
}

func TestParseCoverageOut_Malformed(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"missing-mode-line", "garbage\n"},
		{"missing-colon", "mode: atomic\nfooo 1 2\n"},
		{"bad-field-count", "mode: atomic\nfoo/bar.go:1.1,2.2 BAD\n"},
		{"bad-statement-count", "mode: atomic\nfoo/bar.go:1.1,2.2 not-a-num 1\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), c.name+".out")
			if err := os.WriteFile(path, []byte(c.content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := parseCoverage(path); err == nil {
				t.Errorf("expected error for %s", c.name)
			}
		})
	}
}

func TestAllowlist_LoadsAndIgnoresComments(t *testing.T) {
	got, err := loadAllowlist("testdata/allowlist.txt")
	if err != nil {
		t.Fatalf("loadAllowlist: %v", err)
	}
	want := map[string]bool{
		"github.com/proxa-server/proxa/internal/web": true,
		"github.com/proxa-server/proxa/pkg/types":    true,
	}
	if len(got) != len(want) {
		t.Errorf("got %d entries, want %d", len(got), len(want))
	}
	for k := range want {
		if !got[k] {
			t.Errorf("missing entry %q", k)
		}
	}
}

func TestAllowlist_MissingFileIsNotAnError(t *testing.T) {
	got, err := loadAllowlist(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Errorf("expected nil error for missing allowlist; got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty set; got %v", got)
	}
}

func TestThreshold_HighlightsBelow(t *testing.T) {
	pkgs := map[string]pkgCoverage{
		"a/low":  {covered: 1, total: 10}, // 10%
		"b/high": {covered: 9, total: 10}, // 90%
	}
	r := buildReport(pkgs, 60.0, map[string]bool{})
	// a/low should be marked below-threshold.
	var foundLow, foundHigh reportEntry
	for _, e := range r.Packages {
		switch e.Path {
		case "a/low":
			foundLow = e
		case "b/high":
			foundHigh = e
		}
	}
	if !foundLow.BelowThreshold {
		t.Errorf("expected a/low below threshold; got %+v", foundLow)
	}
	if foundHigh.BelowThreshold {
		t.Errorf("expected b/high above threshold; got %+v", foundHigh)
	}
	if r.BelowThresholdCount != 1 {
		t.Errorf("expected BelowThresholdCount=1, got %d", r.BelowThresholdCount)
	}
}

func TestThreshold_AllowlistedNotHighlighted(t *testing.T) {
	pkgs := map[string]pkgCoverage{
		"a/exempt": {covered: 1, total: 100}, // 1%, would be below
	}
	r := buildReport(pkgs, 60.0, map[string]bool{"a/exempt": true})
	if len(r.Packages) != 1 {
		t.Fatalf("expected 1 package, got %d", len(r.Packages))
	}
	e := r.Packages[0]
	if !e.Allowlisted {
		t.Errorf("expected Allowlisted=true; got %+v", e)
	}
	if e.BelowThreshold {
		t.Errorf("allowlisted package should not be marked below-threshold; got %+v", e)
	}
	if r.BelowThresholdCount != 0 {
		t.Errorf("expected BelowThresholdCount=0; got %d", r.BelowThresholdCount)
	}
}

func TestOutput_JSONMode(t *testing.T) {
	pkgs := map[string]pkgCoverage{
		"a/pkg": {covered: 5, total: 10},
	}
	r := buildReport(pkgs, 60.0, map[string]bool{})
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	// Round-trip to verify the schema.
	var rt report
	if err := json.Unmarshal(b, &rt); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if rt.ThresholdPercent != 60.0 {
		t.Errorf("threshold_percent = %g, want 60", rt.ThresholdPercent)
	}
	if rt.PackageCount != 1 {
		t.Errorf("package_count = %d, want 1", rt.PackageCount)
	}
	if rt.OverallPercent != 50.0 {
		t.Errorf("overall_percent = %g, want 50", rt.OverallPercent)
	}
	// Verify the JSON has the expected key names from the spec.
	for _, key := range []string{`"threshold_percent"`, `"overall_percent"`, `"package_count"`, `"below_threshold_count"`, `"allowlisted_count"`, `"packages"`} {
		if !strings.Contains(string(b), key) {
			t.Errorf("expected JSON to contain %s; got: %s", key, b)
		}
	}
}

func TestOutput_TextMode_BelowThresholdMarker(t *testing.T) {
	pkgs := map[string]pkgCoverage{
		"a/low":   {covered: 1, total: 10},
		"b/high":  {covered: 9, total: 10},
		"c/empty": {covered: 0, total: 4},
	}
	r := buildReport(pkgs, 60.0, map[string]bool{"c/empty": true})
	var buf bytes.Buffer
	writeText(&buf, r)
	out := buf.String()
	// a/low must be prefixed with the warning marker.
	if !strings.Contains(out, "⚠ a/low") {
		t.Errorf("expected ⚠ a/low in output; got:\n%s", out)
	}
	// c/empty must be prefixed with the allowlisted marker.
	if !strings.Contains(out, "~ c/empty") {
		t.Errorf("expected ~ c/empty in output; got:\n%s", out)
	}
	// b/high must not have either marker.
	if strings.Contains(out, "⚠ b/high") || strings.Contains(out, "~ b/high") {
		t.Errorf("expected no marker on b/high; got:\n%s", out)
	}
}

// TestExit_AlwaysZeroInV042 documents the v0.4.2 contract: the binary
// always exits 0 when input is valid. Verifying this directly requires
// invoking main(), which calls os.Exit and would terminate the test
// process. Instead we assert the LOGIC by inspecting that buildReport
// never panics + that ToReport for any input yields a struct. The end-
// to-end exit-0 behavior is covered by `make cover` in CI which runs
// the binary against the real repo coverage data.
func TestExit_AlwaysZeroInV042(t *testing.T) {
	// Sanity: buildReport with various inputs never panics.
	cases := []map[string]pkgCoverage{
		{},
		{"a": {covered: 0, total: 0}},
		{"a": {covered: 100, total: 100}},
		{"a": {covered: 50, total: 50}, "b": {covered: 0, total: 100}},
	}
	for i, pkgs := range cases {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("case %d panicked: %v", i, r)
			}
		}()
		_ = buildReport(pkgs, 60.0, nil)
	}
	// And the empty-allowlist path returns no error.
	if _, err := loadAllowlist(filepath.Join(t.TempDir(), "absent")); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Errorf("loadAllowlist on missing file returned error: %v", err)
	}
}
