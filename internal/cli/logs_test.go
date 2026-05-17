package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/proxa-server/proxa/internal/config"
)

func TestRunLogsFlagValidation(t *testing.T) {
	cases := []struct {
		name      string
		flags     logsFlags
		wantMatch string
	}{
		{"tail too negative", logsFlags{Tail: -2, Project: "default"}, "--tail value -2"},
		{"replica negative", logsFlags{Tail: -1, Replica: -3, Project: "default"}, "--replica value -3"},
		{"project empty", logsFlags{Tail: -1, Project: ""}, "--project"},
		{"since unparseable", logsFlags{Tail: -1, Project: "default", SinceRaw: "broken"}, `--since value "broken"`},
		{"since negative", logsFlags{Tail: -1, Project: "default", SinceRaw: "-5m"}, "must be a positive duration"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := runLogs(context.Background(), &config.Config{DataDir: t.TempDir()}, "svc", tt.flags)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantMatch) {
				t.Errorf("err = %q, want match %q", err, tt.wantMatch)
			}
		})
	}
}

func TestParseSinceFlag(t *testing.T) {
	good := []struct {
		raw  string
		want time.Duration
	}{
		{"5m", 5 * time.Minute},
		{"1h30m", 90 * time.Minute},
		{"1h", time.Hour},
		{"60s", 60 * time.Second},
		{"48h", 48 * time.Hour},
	}
	for _, tt := range good {
		t.Run("ok "+tt.raw, func(t *testing.T) {
			t0 := time.Now()
			got, err := parseSinceFlag(tt.raw)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			// Should be approximately t0 - want; tolerate 1s skew.
			delta := t0.Sub(got)
			if delta < tt.want-time.Second || delta > tt.want+time.Second {
				t.Errorf("delta = %v, want ~%v", delta, tt.want)
			}
		})
	}

	bad := []string{"", "broken", "5", "-1m", "5x"}
	for _, raw := range bad {
		t.Run("bad "+raw, func(t *testing.T) {
			if _, err := parseSinceFlag(raw); err == nil {
				t.Errorf("expected error for %q, got nil", raw)
			}
		})
	}
}
