package toml

import (
	"errors"
	"strings"
	"testing"

	"github.com/proxa-server/proxa/internal/version"
)

func TestParse_Meta_VersionGate_BinaryNewerThanRequired_OK(t *testing.T) {
	withBinaryVersion(t, "v0.5.0")
	doc := `
project = "default"
name = "api"
image = "nginx:1.27"
replicas = 1
[meta]
proxa_version = "0.4.3"
[health]
path = "/"
port = 80
interval = "5s"
timeout = "2s"
retries = 3
`
	res, err := Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
	if res.TaskDef.Meta.ProxaVersion != "0.4.3" {
		t.Errorf("meta.proxa_version round-trip lost: got %q", res.TaskDef.Meta.ProxaVersion)
	}
	if len(res.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", res.Warnings)
	}
}

func TestParse_Meta_VersionGate_BinaryTooOld_Rejected(t *testing.T) {
	withBinaryVersion(t, "v0.4.2")
	doc := `
project = "default"
name = "api"
image = "nginx:1.27"
replicas = 1
[meta]
proxa_version = "0.4.3"
[health]
path = "/"
port = 80
interval = "5s"
timeout = "2s"
retries = 3
`
	_, err := Parse(strings.NewReader(doc))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "requires Proxa 0.4.3+") {
		t.Errorf("error did not mention required version: %v", err)
	}
}

func TestParse_Meta_VersionGate_DevBinary_NeverGated(t *testing.T) {
	withBinaryVersion(t, "dev")
	doc := `
project = "default"
name = "api"
image = "nginx:1.27"
replicas = 1
[meta]
proxa_version = "99.99.99"
[health]
path = "/"
port = 80
interval = "5s"
timeout = "2s"
retries = 3
`
	if _, err := Parse(strings.NewReader(doc)); err != nil {
		t.Errorf("dev binary should never be gated, got %v", err)
	}
}

func TestParse_Meta_VersionGate_EmptyMeta_NoGate(t *testing.T) {
	withBinaryVersion(t, "v0.0.1")
	doc := `
project = "default"
name = "api"
image = "nginx:1.27"
replicas = 1
[health]
path = "/"
port = 80
interval = "5s"
timeout = "2s"
retries = 3
`
	if _, err := Parse(strings.NewReader(doc)); err != nil {
		t.Errorf("empty meta should not gate, got %v", err)
	}
}

func TestParse_UnknownField_WarnsNotErrors(t *testing.T) {
	withBinaryVersion(t, "dev")
	doc := `
project = "default"
name = "api"
image = "nginx:1.27"
replicas = 1
gadget = "this field does not exist"
[health]
path = "/"
port = 80
interval = "5s"
timeout = "2s"
retries = 3
[plugin_xyz]
hello = "world"
`
	res, err := Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("unknown fields must not be fatal, got %v", err)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("expected at least one warning")
	}
	joined := strings.Join(res.Warnings, "\n")
	for _, want := range []string{"gadget", "plugin_xyz"} {
		if !strings.Contains(joined, want) {
			t.Errorf("warnings should name %q; got:\n%s", want, joined)
		}
	}
}

func TestParse_Meta_PrereleaseStripped(t *testing.T) {
	withBinaryVersion(t, "v0.4.3-rc.1")
	doc := `
project = "default"
name = "api"
image = "nginx:1.27"
replicas = 1
[meta]
proxa_version = "0.4.3"
[health]
path = "/"
port = 80
interval = "5s"
timeout = "2s"
retries = 3
`
	if _, err := Parse(strings.NewReader(doc)); err != nil {
		t.Errorf("rc binary at same MAJOR.MINOR.PATCH as required must satisfy gate, got %v", err)
	}
}

func TestParse_DecodeError_NotShadowedByGate(t *testing.T) {
	withBinaryVersion(t, "v0.4.3")
	doc := `this is not valid toml ===`
	_, err := Parse(strings.NewReader(doc))
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Errorf("expected decode error, got %v", err)
	}
	// Sanity: validate that it's not a meta-gate error.
	if err != nil && strings.Contains(err.Error(), "meta.proxa_version") {
		t.Errorf("decode error should not be reported as gate error: %v", err)
	}
	// errors.Is to *any* sentinel is not used here — the parser does not
	// expose a typed error for decode failures.
	_ = errors.Is
}

// withBinaryVersion stomps version.Version for the duration of the test.
// Reverted via t.Cleanup so adjacent tests aren't affected.
func withBinaryVersion(t *testing.T, v string) {
	t.Helper()
	old := version.Version
	version.Version = v
	t.Cleanup(func() { version.Version = old })
}
