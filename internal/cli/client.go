package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/proxa-server/proxa/pkg/types"
)

// Client is the CLI's wrapper around the Proxa HTTP API. Dials Unix
// sockets when ListenAddr starts with unix://, otherwise TCP.
type Client struct {
	listenAddr string
	token      string
	http       *http.Client
}

// NewClient builds a Client for the given listen address. Token is
// read from PROXA_TOKEN env var or ${dataDir}/token file.
func NewClient(listenAddr, dataDir string) (*Client, error) {
	tok := os.Getenv("PROXA_TOKEN")
	if tok == "" {
		b, err := os.ReadFile(dataDir + "/token")
		if err != nil {
			return nil, fmt.Errorf("cli: read bootstrap token: %w", err)
		}
		tok = strings.TrimSpace(string(b))
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	if strings.HasPrefix(listenAddr, "unix://") {
		path := strings.TrimPrefix(listenAddr, "unix://")
		httpClient.Transport = &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", path)
			},
		}
	}

	return &Client{
		listenAddr: listenAddr,
		token:      tok,
		http:       httpClient,
	}, nil
}

func (c *Client) baseURL() string {
	if strings.HasPrefix(c.listenAddr, "unix://") {
		return "http://proxa-unix" // host is irrelevant for Unix-socket dialer
	}
	return strings.Replace(c.listenAddr, "tcp://", "http://", 1)
}

func (c *Client) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var reqBody *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("cli: marshal body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL()+path, requestBody(reqBody))
	if err != nil {
		return nil, fmt.Errorf("cli: new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cli: cannot reach proxa server at %s: %w", c.listenAddr, err)
	}
	return resp, nil
}

func requestBody(b *bytes.Reader) *bytes.Reader {
	if b == nil {
		return bytes.NewReader(nil)
	}
	return b
}

// Scale sets a service's desired replica count via
// POST /api/v1/projects/{project}/services/{name}/scale.
// Idempotent: returns nil if the service does not exist.
func (c *Client) Scale(ctx context.Context, project, name string, replicas int) error {
	path := fmt.Sprintf("/api/v1/projects/%s/services/%s/scale", project, name)
	body := map[string]int{"replicas": replicas}
	resp, err := c.do(ctx, http.MethodPost, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil // idempotent down on missing service
	}
	if resp.StatusCode/100 != 2 {
		return decodeAPIError(resp)
	}
	return nil
}

// SystemStatus fetches GET /api/v1/system/status and decodes it into
// the loose-typed map shape that the JSON API returns.
func (c *Client) SystemStatus(ctx context.Context) (map[string]any, error) {
	resp, err := c.do(ctx, http.MethodGet, "/api/v1/system/status", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, decodeAPIError(resp)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("cli: decode status: %w", err)
	}
	return out, nil
}

// UpsertService PUTs a TaskDef to the API and returns the resulting Service.
func (c *Client) UpsertService(ctx context.Context, td types.TaskDef) (*types.Service, error) {
	path := fmt.Sprintf("/api/v1/projects/%s/services/%s", td.Project, td.Name)
	resp, err := c.do(ctx, http.MethodPut, path, td)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, decodeAPIError(resp)
	}
	var svc types.Service
	if err := json.NewDecoder(resp.Body).Decode(&svc); err != nil {
		return nil, fmt.Errorf("cli: decode service: %w", err)
	}
	return &svc, nil
}

func decodeAPIError(resp *http.Response) error {
	var env struct{ Error, Message string }
	_ = json.NewDecoder(resp.Body).Decode(&env)
	if env.Message == "" {
		env.Message = "HTTP " + resp.Status
	}
	return fmt.Errorf("api[%s]: %s", env.Error, env.Message)
}
