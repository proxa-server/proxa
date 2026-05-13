package types

// Subject is an authenticated identity: a human, an OIDC principal, or a
// service account. Subjects are produced by [Authenticator] implementations
// in internal/auth.
type Subject struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Email    string            `json:"email,omitempty"`
	Provider string            `json:"provider"` // local | oidc:<issuer> | token | agent
	Metadata map[string]string `json:"metadata,omitempty"`
}
