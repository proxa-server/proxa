# Contract: REST API (`/api/v1`)

The Proxa control-plane API. Hosted by `proxa server`. Listens on a Unix socket by default; opt-in TCP listener.

## Conventions

- Base path: `/api/v1` (versioned in URL; v2 will live at `/api/v2`).
- Content-Type: `application/json` (request and response).
- Authentication: `Authorization: Bearer <token>` on every request. Bootstrap token from `proxa init` initially; future-feature OIDC/local password yields different tokens with the same wire format.
- Errors: JSON `{"error": "<machine code>", "message": "<human text>"}` with appropriate HTTP status. `error` codes are stable; `message` may change.
- Pagination: not in v0.0 (scale ≤200 services per node makes it unnecessary). Will be added when needed.
- Idempotency: PUT-style upserts use the resource's natural key in the URL; safe to retry.

## Endpoints in v0.0

### `GET /api/v1/system/status`

Aggregate status — used by `proxa ps`.

**Response (200)**:
```json
{
  "node": {
    "id": "node-local",
    "status": "ready",
    "containerCount": 12,
    "uptime": "3h12m"
  },
  "projects": [
    {
      "name": "default",
      "services": [
        {
          "name": "web",
          "image": "nginx:alpine",
          "desiredReplicas": 3,
          "actualReplicas": 3,
          "status": "healthy"
        }
      ]
    }
  ]
}
```

### `GET /api/v1/projects`

List projects the caller can read.

**Response (200)**: `[{"name": "default", "createdAt": "..."}, ...]`

### `POST /api/v1/projects`

Create a project. Admin only.

**Request body**: `{"name": "<slug>"}`. Validated against `^[a-z0-9][a-z0-9-]{0,62}$`.

**Response**: `201` with the created project; `409 already-exists`; `403 forbidden`.

### `PUT /api/v1/projects/{project}/services/{name}`

Upsert a service spec. The body is the JSON encoding of `types.TaskDef`. The path's `{project}` and `{name}` MUST match the body's `project` and `name` fields (else 400).

This is the endpoint `proxa up <file>` calls after parsing TOML.

**Request body**: a `types.TaskDef` JSON.

**Response**:
- `200` if the service already existed and was updated. Body: the resulting `types.Service`.
- `201` if newly created. Body: the resulting `types.Service`.
- `400 invalid-spec` with details on which field failed validation.
- `403 forbidden`.

The reconciler is *poked* (channel send) on success so it doesn't have to wait for the next tick.

### `GET /api/v1/projects/{project}/services`

List services in a project.

**Response (200)**: `[types.Service, ...]`.

### `GET /api/v1/projects/{project}/services/{name}`

Read one service.

**Response**: `200 types.Service` or `404 not-found`.

### `POST /api/v1/projects/{project}/services/{name}/scale`

Set the desired replica count without re-uploading the spec. Used by `proxa down <service>` (which sets `replicas: 0`).

**Request body**: `{"replicas": <int>}`.

**Response**: `200 types.Service` (with the new desired count); `404 not-found`; `400 invalid` (negative replicas).

### `DELETE /api/v1/projects/{project}/services/{name}`

Remove a service. Reconciler removes its containers on next tick.

**Response**: `204 no-content`; `404 not-found`.

## Wire format detail: how Service serializes

The on-the-wire JSON for `Service` matches `pkg/types/Service`'s JSON tags. Key fields the CLI uses:

```json
{
  "id": "...",
  "project": "default",
  "name": "web",
  "spec": { ...TaskDef... },
  "status": "healthy",
  "replicas": [
    {"id": "abc...", "nodeId": "node-local", "phase": "running", "healthOk": true, ...}
  ],
  "history": [...],
  "createdAt": "2026-05-14T...",
  "updatedAt": "2026-05-14T..."
}
```

`history` is capped at 20 entries; oldest dropped on each new deployment.

## Authentication middleware

Every `/api/v1/*` request passes through:

1. Extract `Authorization: Bearer <token>`.
2. Resolve via `auth.Chain([TokenAuthenticator, LocalPasswordAuthenticator])`. (For v0.0 the password authenticator returns `ErrUnauthenticated` for the bearer flow — only the token one matches. The chain is in place for the future dashboard's `POST /login` endpoint.)
3. On success, the request handler receives the `*types.Subject` via `context.Context`.
4. The handler then calls `policy.Authorize(ctx, AuthzRequest{...})` before doing work. v0.0's `noopPolicyEngine` was replaced earlier in this feature with a real implementation in `internal/auth/policy.go` that consults SQLite-stored policies.

Health-check / liveness endpoints (none in v0.0; added in 002) would bypass auth.

## What's not in v0.0

Reserved for later features:

- `POST /api/v1/auth/login` (local password → token) — added when the dashboard arrives (Feature 004).
- `/api/v1/secrets` — Feature 005.
- `/api/v1/configs` — Feature 006.
- `/api/v1/ingress` — Feature 003.
- `/api/v1/nodes` (other than `system/status`) — added when multi-node ships.
- WebSocket `/api/v1/services/{...}/logs` — Feature 002 or part of dashboard.
- gRPC for control-plane↔agent — Feature 002+.
