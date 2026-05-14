package docker

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"

	"github.com/proxa-server/proxa/internal/runtime"
)

// imageClient is the subset of docker/docker/client used for images.
type imageClient interface {
	ImagePull(ctx context.Context, ref string, options image.PullOptions) (io.ReadCloser, error)
	ImageInspect(ctx context.Context, imageID string, opts ...client.ImageInspectOption) (image.InspectResponse, error)
}

// PullImage pulls an image from a registry. Drains the pull stream
// (which is a sequence of progress JSON frames) and returns nil on
// success.
func (r *Runtime) PullImage(ctx context.Context, ref string) error {
	rc, err := r.cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("runtime/docker: pull %q: %w", ref, err)
	}
	defer rc.Close()
	if _, err := io.Copy(io.Discard, rc); err != nil {
		return fmt.Errorf("runtime/docker: drain pull stream for %q: %w", ref, err)
	}
	return nil
}

// InspectImage returns metadata about an image stored locally.
func (r *Runtime) InspectImage(ctx context.Context, ref string) (*runtime.ImageInfo, error) {
	info, err := r.cli.ImageInspect(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("runtime/docker: inspect image %q: %w", ref, err)
	}
	createdAt, _ := time.Parse(time.RFC3339Nano, info.Created)
	digest := ""
	if len(info.RepoDigests) > 0 {
		digest = info.RepoDigests[0]
	}
	return &runtime.ImageInfo{
		Ref:     ref,
		Digest:  digest,
		Size:    info.Size,
		Created: createdAt,
	}, nil
}
