package docker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/system"
	"github.com/docker/docker/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/proxa-server/proxa/internal/runtime"
)

// mockDockerClient records ContainerCreate inputs so tests can assert
// that the runtime wired the right labels, security opts, etc.
type mockDockerClient struct {
	createCalls    []mockCreateCall
	listResponse   []container.Summary
	listCalls      []container.ListOptions
	startCalledFor []string

	// logs wiring (used by logs_test.go)
	logsBody []byte
	logsErr  error

	// exec wiring (used by exec_test.go)
	execCreateCalls  []mockExecCreateCall
	execAttachResp   string // bytes the mock streams back (TTY-mode plain)
	execAttachStdout string // stdout half for multiplexed mode
	execAttachStderr string // stderr half for multiplexed mode
	execAttachErr    error
	execInspectExit  int
	execInspectErr   error
}

type mockExecCreateCall struct {
	containerID string
	opts        container.ExecOptions
}

type mockCreateCall struct {
	cfg  *container.Config
	host *container.HostConfig
	name string
}

// implement dockerClient
func (m *mockDockerClient) ContainerCreate(ctx context.Context, cfg *container.Config, host *container.HostConfig,
	net *network.NetworkingConfig, platform *ocispec.Platform, name string) (container.CreateResponse, error) {
	m.createCalls = append(m.createCalls, mockCreateCall{cfg: cfg, host: host, name: name})
	return container.CreateResponse{ID: "fake-id-" + name}, nil
}
func (m *mockDockerClient) ContainerStart(ctx context.Context, id string, opts container.StartOptions) error {
	m.startCalledFor = append(m.startCalledFor, id)
	return nil
}
func (m *mockDockerClient) ContainerStop(context.Context, string, container.StopOptions) error { return nil }
func (m *mockDockerClient) ContainerRemove(context.Context, string, container.RemoveOptions) error { return nil }
func (m *mockDockerClient) ContainerRename(context.Context, string, string) error { return nil }
func (m *mockDockerClient) ContainerInspect(context.Context, string) (container.InspectResponse, error) {
	return container.InspectResponse{}, errors.New("not implemented in mock")
}
func (m *mockDockerClient) ContainerList(ctx context.Context, opts container.ListOptions) ([]container.Summary, error) {
	m.listCalls = append(m.listCalls, opts)
	return m.listResponse, nil
}
func (m *mockDockerClient) ImagePull(context.Context, string, image.PullOptions) (io.ReadCloser, error) {
	return io.NopCloser(emptyReader{}), nil
}
func (m *mockDockerClient) ImageInspect(context.Context, string, ...client.ImageInspectOption) (image.InspectResponse, error) {
	return image.InspectResponse{}, errors.New("not implemented in mock")
}
func (m *mockDockerClient) ServerVersion(context.Context) (types.Version, error) {
	return types.Version{Version: "test"}, nil
}
func (m *mockDockerClient) Info(context.Context) (system.Info, error) { return system.Info{}, nil }
func (m *mockDockerClient) Close() error                              { return nil }

// containerLogsBody is what the mock returns from ContainerLogs.
// Tests can stuff pre-built stdcopy-framed bytes here.
func (m *mockDockerClient) ContainerLogs(_ context.Context, _ string, _ container.LogsOptions) (io.ReadCloser, error) {
	if m.logsErr != nil {
		return nil, m.logsErr
	}
	if m.logsBody == nil {
		return io.NopCloser(bytes.NewReader(nil)), nil
	}
	return io.NopCloser(bytes.NewReader(m.logsBody)), nil
}

func (m *mockDockerClient) ContainerExecCreate(_ context.Context, id string, opts container.ExecOptions) (container.ExecCreateResponse, error) {
	m.execCreateCalls = append(m.execCreateCalls, mockExecCreateCall{containerID: id, opts: opts})
	return container.ExecCreateResponse{ID: "exec-" + id}, nil
}

func (m *mockDockerClient) ContainerExecAttach(_ context.Context, _ string, opts container.ExecAttachOptions) (types.HijackedResponse, error) {
	if m.execAttachErr != nil {
		return types.HijackedResponse{}, m.execAttachErr
	}
	// Build a stream the runtime will demultiplex. In multiplexed mode
	// we need stdcopy framing; in TTY mode raw bytes.
	var payload []byte
	if opts.Tty {
		payload = []byte(m.execAttachResp)
	} else {
		payload = appendStdcopyFrame(nil, 1, []byte(m.execAttachStdout))
		payload = appendStdcopyFrame(payload, 2, []byte(m.execAttachStderr))
	}
	hr := types.NewHijackedResponse(&fakeHijackConn{r: bytes.NewReader(payload)}, "")
	return hr, nil
}

func (m *mockDockerClient) ContainerExecInspect(context.Context, string) (container.ExecInspect, error) {
	if m.execInspectErr != nil {
		return container.ExecInspect{}, m.execInspectErr
	}
	return container.ExecInspect{ExitCode: m.execInspectExit, Running: false}, nil
}

// appendStdcopyFrame adds one frame in the format expected by
// pkg/stdcopy.StdCopy: 8-byte header (stream, 0, 0, 0, big-endian len)
// followed by `data`. stream=1 → stdout, 2 → stderr.
func appendStdcopyFrame(buf []byte, stream byte, data []byte) []byte {
	hdr := [8]byte{stream, 0, 0, 0, 0, 0, 0, 0}
	hdr[4] = byte(len(data) >> 24)
	hdr[5] = byte(len(data) >> 16)
	hdr[6] = byte(len(data) >> 8)
	hdr[7] = byte(len(data))
	buf = append(buf, hdr[:]...)
	buf = append(buf, data...)
	return buf
}

// fakeHijackConn is the minimal net.Conn that types.HijackedResponse
// needs for a one-shot read.
type fakeHijackConn struct {
	r io.Reader
}

func (c *fakeHijackConn) Read(p []byte) (int, error)       { return c.r.Read(p) }
func (c *fakeHijackConn) Write(p []byte) (int, error)      { return len(p), nil }
func (c *fakeHijackConn) Close() error                     { return nil }
func (c *fakeHijackConn) LocalAddr() net.Addr              { return fakeAddr{} }
func (c *fakeHijackConn) RemoteAddr() net.Addr             { return fakeAddr{} }
func (c *fakeHijackConn) SetDeadline(t time.Time) error    { return nil }
func (c *fakeHijackConn) SetReadDeadline(time.Time) error  { return nil }
func (c *fakeHijackConn) SetWriteDeadline(time.Time) error { return nil }

type fakeAddr struct{}

func (fakeAddr) Network() string { return "fake" }
func (fakeAddr) String() string  { return "fake" }

type emptyReader struct{}

func (emptyReader) Read(p []byte) (int, error) { return 0, io.EOF }

// runtimeWithMock returns a *Runtime backed by mock.
func runtimeWithMock(m *mockDockerClient) *Runtime {
	return &Runtime{cli: m, nodeID: "node-test"}
}

func TestCreateContainerStampsLabelsAndSecurity(t *testing.T) {
	m := &mockDockerClient{}
	r := runtimeWithMock(m)

	spec := runtime.ContainerSpec{
		Name:  "proxa-default-web-0",
		Image: "nginx:alpine",
		Labels: map[string]string{
			LabelProject:  "default",
			LabelService:  "web",
			LabelReplica:  "0",
			LabelSpecHash: "sha256:test",
		},
	}
	id, err := r.CreateContainer(context.Background(), spec)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if id != "fake-id-proxa-default-web-0" {
		t.Errorf("ID = %q, want fake-id-proxa-default-web-0", id)
	}
	if len(m.createCalls) != 1 {
		t.Fatalf("expected 1 ContainerCreate call, got %d", len(m.createCalls))
	}
	got := m.createCalls[0]

	// FR-002 default user
	if got.cfg.User != "1000:1000" {
		t.Errorf("Config.User = %q, want '1000:1000'", got.cfg.User)
	}
	// Labels stamped
	if got.cfg.Labels[LabelManaged] != "true" {
		t.Errorf("LabelManaged not set: %v", got.cfg.Labels)
	}
	if got.cfg.Labels[LabelProject] != "default" {
		t.Errorf("LabelProject = %q", got.cfg.Labels[LabelProject])
	}
	if got.cfg.Labels[LabelNodeID] != "node-test" {
		t.Errorf("LabelNodeID = %q, want 'node-test'", got.cfg.Labels[LabelNodeID])
	}
	// Security defaults
	if len(got.host.CapDrop) == 0 || got.host.CapDrop[0] != "ALL" {
		t.Errorf("CapDrop missing ALL: %v", got.host.CapDrop)
	}
	if !containsString(got.host.SecurityOpt, "no-new-privileges:true") {
		t.Errorf("SecurityOpt missing no-new-privileges: %v", got.host.SecurityOpt)
	}
}

func TestListContainersRequiresProject(t *testing.T) {
	m := &mockDockerClient{}
	r := runtimeWithMock(m)

	_, err := r.ListContainers(context.Background(), runtime.ListFilter{})
	if err == nil {
		t.Errorf("ListContainers with empty project should error")
	}
	if len(m.listCalls) != 0 {
		t.Errorf("Docker should not have been called")
	}
}

func TestListContainersFiltersByProjectLabel(t *testing.T) {
	m := &mockDockerClient{
		listResponse: []container.Summary{{ID: "a", Names: []string{"/proxa-default-web-0"}, Image: "nginx:alpine", State: "running"}},
	}
	r := runtimeWithMock(m)

	out, err := r.ListContainers(context.Background(), runtime.ListFilter{Project: "default"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out) != 1 {
		t.Errorf("expected 1 container, got %d", len(out))
	}
	if len(m.listCalls) != 1 {
		t.Fatalf("expected 1 ContainerList call")
	}
	args := m.listCalls[0].Filters
	if !args.Match("label", LabelManaged+"=true") {
		t.Errorf("filter missing proxa.managed=true")
	}
	if !args.Match("label", LabelProject+"=default") {
		t.Errorf("filter missing proxa.project=default")
	}
}
