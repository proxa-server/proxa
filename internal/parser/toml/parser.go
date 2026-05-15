// Package toml parses Proxa TaskDef TOML files into pkg/types.TaskDef.
// See specs/001-core-loop/contracts/toml-grammar.md for the grammar and
// validation rules.
package toml

import (
	"fmt"
	"io"

	"github.com/BurntSushi/toml"

	"github.com/proxa-server/proxa/pkg/types"
)

// Parse reads a TaskDef TOML document from r. Sets Project="default"
// when absent. Validates per [Validate] before returning.
func Parse(r io.Reader) (types.TaskDef, error) {
	var td types.TaskDef
	if _, err := toml.NewDecoder(r).Decode(&td); err != nil {
		return td, fmt.Errorf("parser/toml: decode: %w", err)
	}
	if td.Project == "" {
		td.Project = "default"
	}
	if err := Validate(td); err != nil {
		return td, err
	}
	return td, nil
}
