package types

import "time"

// Project is the logical grouping that scopes every other resource
// (constitution §III). Projects are API-created, not TOML-declared, so
// only JSON tags are needed.
type Project struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}
