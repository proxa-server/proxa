package server

import (
	"bufio"
	"errors"
	"fmt"
	"net/http"
	"cmp"
	"slices"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/proxa-server/proxa/internal/auth"
	rt "github.com/proxa-server/proxa/internal/runtime"
	dockerlabels "github.com/proxa-server/proxa/internal/runtime/docker"
)

// handleStreamServiceLogs serves GET /api/v1/projects/{project}/services/{name}/logs.
// Order of operations follows specs/004-logs/data-model.md lifecycle:
//   1. parse + validate query params
//   2. project-scoped auth (§III) — NO daemon resource opened until this passes
//   3. resolve replica index → container ID
//   4. write headers + (SSE) meta event
//   5. open Runtime.StreamLogs, copy lines until EOF / ctx cancel
//   6. on EOF write SSE end event or close conn
func (s *Server) handleStreamServiceLogs(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")

	// (1) Parse + validate query params.
	q := r.URL.Query()
	tail, err := parseTail(q.Get("tail"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid-tail", err.Error())
		return
	}
	follow := q.Get("follow") == "true"
	replicaIdx, err := parseReplica(q.Get("replica"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid-replica", err.Error())
		return
	}
	var since time.Time
	if raw := q.Get("since"); raw != "" {
		t, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid-since", fmt.Sprintf("--since %q: %v", raw, err))
			return
		}
		since = t
	}

	// (2) Project-scoped auth. Done BEFORE any daemon call so failed
	// requests cost zero daemon resources (§III + FR-010).
	subject := SubjectFromContext(r.Context())
	if subject == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "no authenticated subject")
		return
	}
	if err := s.authz.Authorize(r.Context(), auth.AuthzRequest{
		Subject: subject,
		Verb:    auth.VerbLogs,
		Resource: auth.ResourceRef{
			Kind:    auth.KindService,
			Project: project,
			Name:    name,
		},
	}); err != nil {
		if errors.Is(err, auth.ErrForbidden) {
			writeError(w, http.StatusForbidden, "logs-cross-project",
				fmt.Sprintf("unauthorized for project %q", project))
			return
		}
		writeError(w, http.StatusInternalServerError, "authz-error", err.Error())
		return
	}

	// (3) Resolve replica index → container ID.
	containerID, containerName, err := s.resolveReplicaContainer(r, project, name, replicaIdx)
	if err != nil {
		code, status := classifyResolveErr(err)
		writeError(w, status, code, err.Error())
		return
	}

	// (4) Write headers. Branch on Accept header for wire format.
	wantSSE := r.Header.Get("Accept") == "text/event-stream"
	if wantSSE {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
	} else {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
	w.Header().Set("X-Proxa-Container", containerName)
	w.Header().Set("X-Proxa-Replica", strconv.Itoa(replicaIdx))
	w.WriteHeader(http.StatusOK)
	flush(w)

	if wantSSE {
		writeSSEEvent(w, "meta", fmt.Sprintf(`{"container":%q,"replica":%d}`, containerName, replicaIdx))
	}

	// (5) Open the daemon stream and copy lines.
	rc, err := s.runtime.StreamLogs(r.Context(), containerID, rt.LogOpts{
		Follow: follow,
		Tail:   tail,
		Since:  since,
	})
	if err != nil {
		// Already wrote 200 headers; report mid-stream.
		if wantSSE {
			writeSSEEvent(w, "error", fmt.Sprintf(`{"code":"runtime-error","error":%q}`, err.Error()))
		} else {
			writePlainLine(w, "proxa: stream error: "+err.Error())
		}
		return
	}
	defer rc.Close()

	scanner := bufio.NewScanner(rc)
	// Container log lines can be long (JSON dumps, stack traces); raise the buffer.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if wantSSE {
			writeSSEData(w, line)
		} else {
			writePlainLine(w, line)
		}
	}

	// (6) EOF / ctx cancel.
	if wantSSE {
		writeSSEEvent(w, "end", `{"reason":"stream closed"}`)
	}
}

// resolveReplicaContainer lists this project's containers, filters by
// service name, sorts by replica label, and returns the (id, name)
// matching replicaIdx. Errors: service-not-found, replica-not-found,
// replica-not-available.
func (s *Server) resolveReplicaContainer(r *http.Request, project, service string, replicaIdx int) (id, name string, err error) {
	containers, err := s.runtime.ListContainers(r.Context(), rt.ListFilter{Project: project})
	if err != nil {
		return "", "", fmt.Errorf("list containers: %w", err)
	}
	matching := []rt.ContainerInfo{}
	for _, c := range containers {
		if c.Labels[dockerlabels.LabelService] == service {
			matching = append(matching, c)
		}
	}
	if len(matching) == 0 {
		return "", "", errServiceNotFound{Project: project, Service: service}
	}
	// Sort by replica index ascending.
	slices.SortFunc(matching, func(a, b rt.ContainerInfo) int {
		ra, _ := strconv.Atoi(a.Labels[dockerlabels.LabelReplica])
		rb, _ := strconv.Atoi(b.Labels[dockerlabels.LabelReplica])
		return cmp.Compare(ra, rb)
	})
	if replicaIdx >= len(matching) {
		return "", "", errReplicaNotFound{ReplicaIdx: replicaIdx, Have: len(matching)}
	}
	c := matching[replicaIdx]
	if c.State != "running" && c.State != "created" {
		return "", "", errReplicaNotAvailable{ContainerName: c.Name, State: c.State}
	}
	return c.ID, c.Name, nil
}

// --- error types + classification ---

type errServiceNotFound struct{ Project, Service string }

func (e errServiceNotFound) Error() string {
	return fmt.Sprintf("service %q not found in project %q", e.Service, e.Project)
}

type errReplicaNotFound struct{ ReplicaIdx, Have int }

func (e errReplicaNotFound) Error() string {
	return fmt.Sprintf("replica %d not found (service has %d replicas)", e.ReplicaIdx, e.Have)
}

type errReplicaNotAvailable struct{ ContainerName, State string }

func (e errReplicaNotAvailable) Error() string {
	return fmt.Sprintf("replica %s not running (state %q)", e.ContainerName, e.State)
}

func classifyResolveErr(err error) (code string, status int) {
	switch err.(type) {
	case errServiceNotFound:
		return "service-not-found", http.StatusNotFound
	case errReplicaNotFound:
		return "replica-not-found", http.StatusServiceUnavailable
	case errReplicaNotAvailable:
		return "replica-not-available", http.StatusServiceUnavailable
	default:
		return "runtime-error", http.StatusBadGateway
	}
}

// --- query param parsing ---

func parseTail(raw string) (int, error) {
	if raw == "" {
		return -1, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("--tail %q: not an integer", raw)
	}
	if n < -1 {
		return 0, fmt.Errorf("--tail %d: must be >= -1", n)
	}
	return n, nil
}

func parseReplica(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("--replica %q: not an integer", raw)
	}
	if n < 0 {
		return 0, fmt.Errorf("--replica %d: must be >= 0", n)
	}
	return n, nil
}
