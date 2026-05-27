package docker

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/proxa-server/proxa/internal/runtime"
)

// containerClient is the subset of docker/docker/client we use for
// containers.
type containerClient interface {
	ContainerCreate(ctx context.Context, cfg *container.Config, host *container.HostConfig,
		network *network.NetworkingConfig, platform *ocispec.Platform, name string) (container.CreateResponse, error)
	ContainerStart(ctx context.Context, id string, options container.StartOptions) error
	ContainerStop(ctx context.Context, id string, options container.StopOptions) error
	ContainerRestart(ctx context.Context, id string, options container.StopOptions) error
	ContainerRemove(ctx context.Context, id string, options container.RemoveOptions) error
	ContainerRename(ctx context.Context, id, newName string) error
	ContainerInspect(ctx context.Context, id string) (container.InspectResponse, error)
	ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error)
}

// CreateContainer creates a container with §II security defaults applied.
// The container starts in the "created" state — call StartContainer
// separately.
func (r *Runtime) CreateContainer(ctx context.Context, spec runtime.ContainerSpec) (string, error) {
	cfg, host := applySecurityProfile(spec)

	// Stamp the proxa-managed labels (caller passes the per-replica
	// values via spec.Labels — we read them back out here).
	replica, _ := strconv.Atoi(spec.Labels[LabelReplica])
	specHash := spec.Labels[LabelSpecHash]
	cfg.Labels = BuildContainerLabels(spec, replica, specHash, r.nodeID)

	resp, err := r.cli.ContainerCreate(ctx, cfg, host, nil, nil, spec.Name)
	if err != nil {
		return "", fmt.Errorf("runtime/docker: create container %q: %w", spec.Name, err)
	}
	return resp.ID, nil
}

// StartContainer transitions a container to "running".
func (r *Runtime) StartContainer(ctx context.Context, id string) error {
	if err := r.cli.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
		return fmt.Errorf("runtime/docker: start %q: %w", id, err)
	}
	return nil
}

// StopContainer sends SIGTERM, then SIGKILL after gracePeriod.
func (r *Runtime) StopContainer(ctx context.Context, id string, gracePeriod time.Duration) error {
	if gracePeriod <= 0 {
		gracePeriod = 10 * time.Second
	}
	secs := int(gracePeriod.Seconds())
	if err := r.cli.ContainerStop(ctx, id, container.StopOptions{Timeout: &secs}); err != nil {
		return fmt.Errorf("runtime/docker: stop %q: %w", id, err)
	}
	return nil
}

// RestartContainer restarts a container in place (atomic w.r.t. cgroup
// teardown). gracePeriod is the SIGTERM-to-SIGKILL window applied to
// the stop half of the restart; gracePeriod <= 0 falls back to 10s.
func (r *Runtime) RestartContainer(ctx context.Context, id string, gracePeriod time.Duration) error {
	if gracePeriod <= 0 {
		gracePeriod = 10 * time.Second
	}
	secs := int(gracePeriod.Seconds())
	if err := r.cli.ContainerRestart(ctx, id, container.StopOptions{Timeout: &secs}); err != nil {
		return fmt.Errorf("runtime/docker: restart %q: %w", id, err)
	}
	return nil
}

// RemoveContainer removes a container. force=true removes even if running.
func (r *Runtime) RemoveContainer(ctx context.Context, id string, force bool) error {
	err := r.cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: force})
	if err != nil {
		return fmt.Errorf("runtime/docker: remove %q: %w", id, err)
	}
	return nil
}

// RenameContainer changes a container's name. The new name must be
// unique in the daemon's namespace; callers handle collisions.
func (r *Runtime) RenameContainer(ctx context.Context, id, newName string) error {
	if id == "" || newName == "" {
		return fmt.Errorf("runtime/docker: rename: id and newName required")
	}
	if err := r.cli.ContainerRename(ctx, id, newName); err != nil {
		return fmt.Errorf("runtime/docker: rename %q -> %q: %w", id, newName, err)
	}
	return nil
}

// InspectContainer returns metadata about a container by ID or name.
func (r *Runtime) InspectContainer(ctx context.Context, id string) (*runtime.ContainerInfo, error) {
	resp, err := r.cli.ContainerInspect(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("runtime/docker: inspect %q: %w", id, err)
	}
	info := &runtime.ContainerInfo{
		ID:     resp.ID,
		Name:   resp.Name,
		Image:  resp.Config.Image,
		State:  resp.State.Status,
		Labels: resp.Config.Labels,
	}
	if resp.State.Health != nil {
		info.Health = resp.State.Health.Status
	}
	// Resolve the container's bridge-network IPv4 for HTTP probes (R-001).
	// Prefer the "bridge" network; fall back to the first non-empty IP.
	if resp.NetworkSettings != nil {
		if ep, ok := resp.NetworkSettings.Networks["bridge"]; ok && ep != nil && ep.IPAddress != "" {
			info.IPAddress = ep.IPAddress
		} else {
			for _, ep := range resp.NetworkSettings.Networks {
				if ep != nil && ep.IPAddress != "" {
					info.IPAddress = ep.IPAddress
					break
				}
			}
		}
	}
	return info, nil
}

// ListAllContainers returns every container the daemon knows about —
// Proxa-managed AND host. Used by the v0.4.4 Containers dashboard.
// The proxa.project label is included on managed rows; absent on host
// rows. Includes stopped containers (All=true).
func (r *Runtime) ListAllContainers(ctx context.Context) ([]runtime.ContainerInfo, error) {
	summaries, err := r.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("runtime/docker: list all containers: %w", err)
	}
	out := make([]runtime.ContainerInfo, 0, len(summaries))
	for _, s := range summaries {
		name := ""
		if len(s.Names) > 0 {
			name = s.Names[0]
		}
		out = append(out, runtime.ContainerInfo{
			ID:     s.ID,
			Name:   name,
			Image:  s.Image,
			State:  s.State,
			Labels: s.Labels,
		})
	}
	return out, nil
}

// ListContainers returns Proxa-managed containers in the given project.
// REJECTS empty Project per constitution §III.
func (r *Runtime) ListContainers(ctx context.Context, filter runtime.ListFilter) ([]runtime.ContainerInfo, error) {
	if filter.Project == "" {
		return nil, errors.New("runtime/docker: ListContainers requires non-empty Project filter (constitution §III)")
	}
	args := filters.NewArgs()
	args.Add("label", LabelManaged+"=true")
	args.Add("label", LabelProject+"="+filter.Project)
	for k, v := range filter.Labels {
		args.Add("label", k+"="+v)
	}
	summaries, err := r.cli.ContainerList(ctx, container.ListOptions{All: true, Filters: args})
	if err != nil {
		return nil, fmt.Errorf("runtime/docker: list containers in project %q: %w", filter.Project, err)
	}
	out := make([]runtime.ContainerInfo, 0, len(summaries))
	for _, s := range summaries {
		name := ""
		if len(s.Names) > 0 {
			name = s.Names[0]
		}
		out = append(out, runtime.ContainerInfo{
			ID:     s.ID,
			Name:   name,
			Image:  s.Image,
			State:  s.State,
			Labels: s.Labels,
		})
	}
	return out, nil
}
