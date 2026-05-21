package server

import (
	"encoding/json"
	"net/http"

	"github.com/proxa-server/proxa/internal/version"
)

// handleSystemInfo serves GET /api/v1/system — runtime introspection of
// the running proxa server. See contracts/system-info-api.md for the
// payload shape and stability contract.
//
// Scope: NOT project-scoped (per-node metadata). Bearer-token auth
// required via the parent /api/v1 RequireAuth middleware.
func (s *Server) handleSystemInfo(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(version.System()); err != nil {
		writeError(w, http.StatusInternalServerError, "encode-failed", err.Error())
	}
}
