// Package toml parses Proxa TaskDef TOML files into pkg/types.TaskDef.
// See specs/001-core-loop/contracts/toml-grammar.md for the grammar and
// validation rules; see docs/proxa-toml-v1.md for the v1 format spec
// (including the [meta] block + unknown-field warnings, both added in
// v0.4.3).
package toml

import (
	"fmt"
	"io"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/proxa-server/proxa/internal/version"
	"github.com/proxa-server/proxa/pkg/types"
)

// Result is the outcome of a Parse call: the decoded TaskDef plus any
// non-fatal warnings (currently: unknown TOML fields). Warnings should
// be logged by the caller but do NOT fail the parse — the v1 contract
// is that unknown fields are forward-compatible.
type Result struct {
	TaskDef  types.TaskDef
	Warnings []string
}

// Parse reads a TaskDef TOML document from r. Sets Project="default"
// when absent. Validates per [Validate] before returning.
//
// Errors:
//   - decode failure → "parser/toml: decode: ..."
//   - meta.proxa_version requires a binary newer than the running one
//     → "parser/toml: meta.proxa_version=<X> requires Proxa <X>+ but
//     binary is <Y>"
//   - validation failure → returned from [Validate]
//
// Warnings (non-fatal): unknown top-level / table fields. Each warning
// is formatted "parser/toml: unknown field: <dotted.path>".
func Parse(r io.Reader) (Result, error) {
	var td types.TaskDef
	md, err := toml.NewDecoder(r).Decode(&td)
	if err != nil {
		return Result{}, fmt.Errorf("parser/toml: decode: %w", err)
	}
	if td.Project == "" {
		td.Project = "default"
	}

	if err := checkMetaVersion(td.Meta.ProxaVersion, version.Version); err != nil {
		return Result{TaskDef: td}, err
	}

	var warnings []string
	for _, key := range md.Undecoded() {
		warnings = append(warnings, fmt.Sprintf("parser/toml: unknown field: %s", key.String()))
	}

	if err := Validate(td); err != nil {
		return Result{TaskDef: td, Warnings: warnings}, err
	}
	return Result{TaskDef: td, Warnings: warnings}, nil
}

// checkMetaVersion compares declared meta.proxa_version against the
// running binary's version. Returns nil if compatible (binary >= required)
// or if either side is "dev" / empty (dev builds never gate; empty meta
// means the document declares no requirement).
//
// Comparison is a simple lexical semver compare on dot-split numeric
// components — sufficient for the v1 vocabulary (0.4.3, 0.5.0, 1.0.0).
// Pre-release tags ("-rc.1") are stripped before comparison.
func checkMetaVersion(required, binary string) error {
	required = strings.TrimPrefix(required, "v")
	if required == "" {
		return nil
	}
	binary = strings.TrimPrefix(binary, "v")
	if binary == "" || binary == "dev" {
		return nil
	}
	if cmp, ok := semverCompare(binary, required); ok && cmp < 0 {
		return fmt.Errorf("parser/toml: meta.proxa_version=%s requires Proxa %s+ but binary is %s",
			required, required, binary)
	}
	return nil
}

// semverCompare returns (sign, ok) where sign is <0 / 0 / >0 like
// strings.Compare and ok=false signals a non-numeric component (in
// which case the caller should fall back to permissive behavior).
func semverCompare(a, b string) (int, bool) {
	a = stripPrerelease(a)
	b = stripPrerelease(b)
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		ai, aok := atoiSafe(getOrZero(as, i))
		bi, bok := atoiSafe(getOrZero(bs, i))
		if !aok || !bok {
			return 0, false
		}
		if ai != bi {
			if ai < bi {
				return -1, true
			}
			return 1, true
		}
	}
	return 0, true
}

func stripPrerelease(s string) string {
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		return s[:i]
	}
	return s
}

func getOrZero(xs []string, i int) string {
	if i >= len(xs) {
		return "0"
	}
	return xs[i]
}

func atoiSafe(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	return n, true
}
