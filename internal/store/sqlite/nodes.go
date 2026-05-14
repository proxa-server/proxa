package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/proxa-server/proxa/internal/store"
	"github.com/proxa-server/proxa/pkg/types"
)

// PutNode inserts or updates a node row.
func (s *Store) PutNode(ctx context.Context, n types.Node) error {
	if n.ID == "" {
		return fmt.Errorf("store/sqlite: PutNode requires non-empty ID")
	}
	labelsJSON, err := json.Marshal(n.Labels)
	if err != nil {
		return fmt.Errorf("store/sqlite: marshal node labels: %w", err)
	}
	if n.Labels == nil {
		labelsJSON = []byte("{}")
	}
	if n.LastHeartbeat.IsZero() {
		n.LastHeartbeat = time.Now().UTC()
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO nodes(id, name, role, address, cpu_cores, memory_mb, disk_gb, container_count, last_heartbeat, status, labels_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			role = excluded.role,
			address = excluded.address,
			cpu_cores = excluded.cpu_cores,
			memory_mb = excluded.memory_mb,
			disk_gb = excluded.disk_gb,
			container_count = excluded.container_count,
			last_heartbeat = excluded.last_heartbeat,
			status = excluded.status,
			labels_json = excluded.labels_json
	`, n.ID, n.Name, string(n.Role), n.Address,
		n.Resources.CPUCores, n.Resources.MemoryMB, n.Resources.DiskGB,
		n.ContainerCount, n.LastHeartbeat.Format(time.RFC3339), string(n.Status), string(labelsJSON))
	if err != nil {
		return fmt.Errorf("store/sqlite: upsert node %q: %w", n.ID, err)
	}
	return nil
}

// GetNode fetches a node by ID.
func (s *Store) GetNode(ctx context.Context, id string) (*types.Node, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, role, address, cpu_cores, memory_mb, disk_gb, container_count, last_heartbeat, status, labels_json
		FROM nodes WHERE id = ?
	`, id)
	n, err := scanNode(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: get node %q: %w", id, err)
	}
	return n, nil
}

// ListNodes returns every node, sorted by name.
func (s *Store) ListNodes(ctx context.Context) ([]types.Node, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, role, address, cpu_cores, memory_mb, disk_gb, container_count, last_heartbeat, status, labels_json
		FROM nodes ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list nodes: %w", err)
	}
	defer rows.Close()

	var out []types.Node
	for rows.Next() {
		n, err := scanNode(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store/sqlite: scan node: %w", err)
		}
		out = append(out, *n)
	}
	return out, rows.Err()
}

// DeleteNode removes a node row.
func (s *Store) DeleteNode(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM nodes WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store/sqlite: delete node %q: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// Heartbeat is the high-frequency hot path. Single indexed UPDATE, no
// other side effects (per data-model.md note).
func (s *Store) Heartbeat(ctx context.Context, nodeID string, at time.Time) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE nodes SET last_heartbeat = ? WHERE id = ?`,
		at.Format(time.RFC3339), nodeID)
	if err != nil {
		return fmt.Errorf("store/sqlite: heartbeat %q: %w", nodeID, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func scanNode(scan func(...any) error) (*types.Node, error) {
	var (
		n             types.Node
		role, status  string
		lastHB        string
		labelsJSON    string
	)
	if err := scan(&n.ID, &n.Name, &role, &n.Address,
		&n.Resources.CPUCores, &n.Resources.MemoryMB, &n.Resources.DiskGB,
		&n.ContainerCount, &lastHB, &status, &labelsJSON); err != nil {
		return nil, err
	}
	n.Role = types.NodeRole(role)
	n.Status = types.NodeStatus(status)
	if t, err := time.Parse(time.RFC3339, lastHB); err == nil {
		n.LastHeartbeat = t
	}
	if labelsJSON != "" && labelsJSON != "{}" {
		_ = json.Unmarshal([]byte(labelsJSON), &n.Labels)
	}
	return &n, nil
}
