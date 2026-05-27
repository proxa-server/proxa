package version

// ResetDistributionCacheForTest clears the distribution-detector cache
// so the next call to System() re-runs detection. ONLY for use from
// tests — the distribution channel never changes within a real
// process lifetime, so production code should not invoke this.
//
// Exported (vs an internal test helper) because the v0.4.2 test suite
// lives in the version_test package (external test package), which
// can't reach internal symbols. Keeping this in a non-_test.go file
// ALSO works at build time without a build tag.
func ResetDistributionCacheForTest() {
	distributionCache = ""
}
