package events

import "time"

// Event is one record in the events table. See package godoc for the
// design contract.
type Event struct {
	ID      int64     `json:"id"`
	At      time.Time `json:"at"`
	Type    string    `json:"type"`
	Actor   string    `json:"actor"`
	Target  string    `json:"target"`
	Payload string    `json:"payload"` // JSON blob (caller owns the schema per Type)
}

// Type constants. Use these instead of raw strings so renames are
// compiler-checked across the codebase.
const (
	// Reconciler-emitted events
	TypeReconcilerCreate      = "reconciler.create"
	TypeReconcilerRemove      = "reconciler.remove"
	TypeReconcilerStart       = "reconciler.start"
	TypeReconcilerStop        = "reconciler.stop"
	TypeReconcilerScale       = "reconciler.scale"
	TypeServiceStatusChanged  = "service.status_changed"
	TypeProbeTransition       = "probe.transition"

	// User-initiated write events (v0.4.4 Container UI lands these)
	TypeUserContainerStart    = "user.container.start"
	TypeUserContainerStop     = "user.container.stop"
	TypeUserContainerRestart  = "user.container.restart"
	TypeUserContainerRemove   = "user.container.remove"
	TypeUserServiceUpsert     = "user.service.upsert"
	TypeUserServiceDelete     = "user.service.delete"
	TypeUserServiceScale      = "user.service.scale"

	// System events
	TypeSystemBoot     = "system.boot"
	TypeSystemShutdown = "system.shutdown"
)

// Actor prefixes — distinguish system from user actors.
const (
	ActorReconciler = "reconciler"
	ActorSystem     = "system"
	// User actors are formatted as: "subject:<subject-id>"
)

// Target formatters — keep target strings consistent across emitters.
func TargetService(project, name string) string  { return "service:" + project + "/" + name }
func TargetContainer(id string) string           { return "container:" + id }
func TargetNode(id string) string                { return "node:" + id }
func TargetSubject(id string) string             { return "subject:" + id }
