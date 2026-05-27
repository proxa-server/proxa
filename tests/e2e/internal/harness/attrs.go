package harness

import "testing"

// SCAttrs tags a test with its spec + success-criterion identifiers via
// Go 1.25's T.Attr — surfaces in `go test -json` output so coverage
// reports can cross-reference which SC each test validates.
//
// Usage at the top of a test body:
//
//	func TestSC_001_Foo(t *testing.T) {
//	    harness.SCAttrs(t, "006", "SC-001")
//	    // ... assertions ...
//	}
//
// Per FR-007 (006-test-foundation-public-images).
func SCAttrs(t *testing.T, spec, sc string) {
	t.Helper()
	t.Attr("spec", spec)
	t.Attr("sc", sc)
}
