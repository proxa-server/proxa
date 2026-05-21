package types

import "time"

// TaskDef is the parsed representation of a user TOML file describing
// a service or a job. It is the input to the reconciliation loop.
type TaskDef struct {
	Project   string            `toml:"project"   json:"project"`
	Name      string            `toml:"name"      json:"name"`
	Image     string            `toml:"image"     json:"image"`
	Replicas  int               `toml:"replicas"  json:"replicas"`           // services only
	Stateful  bool              `toml:"stateful"  json:"stateful"`           // affects deploy strategy + scheduling
	Security  SecurityProfile   `toml:"security"  json:"security"`           // see security.go
	Env       map[string]string `toml:"env"       json:"env,omitempty"`
	Volumes   []VolumeMount     `toml:"volumes"   json:"volumes,omitempty"`
	Expose    []PortSpec        `toml:"expose"    json:"expose,omitempty"`
	Strategy  DeployStrategy    `toml:"strategy"  json:"strategy"`           // start-first | stop-first
	Health    HealthCheck       `toml:"health"    json:"health"`
	Resources ResourceLimits    `toml:"resources" json:"resources"`
	Schedule  string            `toml:"schedule"  json:"schedule,omitempty"` // cron expr; jobs only
	Routes    []Route           `toml:"route"     json:"routes,omitempty"`   // [[route]] blocks
}

// VolumeMount is a host-path or named-volume mount for a container.
type VolumeMount struct {
	Source   string `toml:"source"   json:"source"`
	Target   string `toml:"target"   json:"target"`
	ReadOnly bool   `toml:"readonly" json:"readOnly"`
}

// PortSpec describes a port exposed by a container. A Host of 0 means
// the port is reachable only via the ingress controller, not bound to
// the host directly.
type PortSpec struct {
	Container int    `toml:"container" json:"container"`
	Host      int    `toml:"host"      json:"host,omitempty"`
	Protocol  string `toml:"protocol"  json:"protocol"` // tcp | udp | http | https
}

// DeployStrategy controls how the reconciler swaps replicas during a
// deployment. Stateless services default to StartFirst (zero-downtime);
// stateful services default to StopFirst (data-safe).
type DeployStrategy string

const (
	StrategyStartFirst DeployStrategy = "start-first"
	StrategyStopFirst  DeployStrategy = "stop-first"
)

// HealthCheck describes how the reconciler probes a container. Either
// Path+Port (HTTP probe) or Command (exec probe) is set; both is invalid.
type HealthCheck struct {
	Path     string        `toml:"path"     json:"path,omitempty"`
	Port     int           `toml:"port"     json:"port,omitempty"`
	Command  []string      `toml:"command"  json:"command,omitempty"`
	Interval time.Duration `toml:"interval" json:"interval"`
	Timeout  time.Duration `toml:"timeout"  json:"timeout"`
	Retries  int           `toml:"retries"  json:"retries"`
	// Via selects the network path the HTTP probe takes:
	//   "" or "direct" → dial the container's bridge IP (default, v0.2 behavior).
	//   "ingress"      → loopback HTTP request through our own ingress with
	//                    Host header injection (works on macOS Docker Desktop
	//                    where bridge IPs are unreachable from the host).
	// Only meaningful for HTTP probes (Path != ""); ignored for exec.
	Via string `toml:"via" json:"via,omitempty"`

	// FollowRedirects controls HTTP redirect handling. Tri-state:
	//   nil   → use the default for the probe's construction mode:
	//           direct probes follow redirects (Go default, up to 10);
	//           via-ingress probes with TLS enabled bypass the redirect
	//           entirely by targeting the HTTPS port directly (the v0.4.1
	//           fix for the probe-via-ingress + TLS=true collision).
	//   *true  → force redirect-following (up to 10); final response status
	//            is the probe outcome.
	//   *false → do not follow redirects; treat 3xx as non-2xx (probe fails).
	//            Useful when the operator wants to assert the redirect itself.
	// Only meaningful for HTTP probes (Path != ""); ignored for exec.
	FollowRedirects *bool `toml:"follow_redirects" json:"follow_redirects,omitempty"`
}

// Route is one operator-declared mapping of (hostname, optional path)
// → this service. Multiple routes per service are allowed (e.g., a
// service serving both api.example.com and admin.example.com).
//
// L7 routes (L4 == "") match by HTTP Host header + URL path-prefix.
// L4 routes (L4 in {"tcp","udp"}) bind the configured Port and
// transparently forward bytes to one of the service's backends.
//
// See specs/003-ingress/contracts/route.md for the full grammar and
// validation rules.
type Route struct {
	Host       string `toml:"host"        json:"host"`                 // FQDN; required for L7
	Path       string `toml:"path"        json:"path,omitempty"`        // prefix; "*" trailing wildcard only
	L4         string `toml:"l4"          json:"l4,omitempty"`          // "" (L7), "tcp", "udp"
	Port       int    `toml:"port"        json:"port,omitempty"`        // L4 only — required for tcp/udp
	LBStrategy string `toml:"lb_strategy" json:"lbStrategy,omitempty"`  // "random" (default), "round-robin"
}

// ResourceLimits are the per-container resource caps passed through to
// the runtime. Strings (rather than typed numbers) match the cgroup-style
// syntax users expect: "500m", "2Gi", etc.
type ResourceLimits struct {
	CPU       string `toml:"cpu"       json:"cpu,omitempty"`
	Memory    string `toml:"memory"    json:"memory,omitempty"`
	PidsLimit int    `toml:"pidsLimit" json:"pidsLimit,omitempty"`
}
