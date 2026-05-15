package cli

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/proxa-server/proxa/internal/auth/password"
	"github.com/proxa-server/proxa/internal/config"
	"github.com/proxa-server/proxa/internal/store/sqlite"
	"github.com/proxa-server/proxa/pkg/types"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize the data directory, SQLite database, master key, and bootstrap credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			return runInit(cmd.Context(), cfg)
		},
	}
}

func runInit(ctx context.Context, cfg *config.Config) error {
	// 1. Create data dir.
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return fmt.Errorf("init: create data dir: %w", err)
	}
	fmt.Printf("created  %s/\n", cfg.DataDir)

	// 2. Open + migrate SQLite.
	st := sqlite.New()
	dbPath := filepath.Join(cfg.DataDir, "proxa.db")
	if err := st.Open(ctx, dbPath); err != nil {
		return err
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		return err
	}
	fmt.Printf("created  %s (SQLite, schema v1)\n", dbPath)

	// 3. Master key for future secrets (stored 0600).
	keyPath := filepath.Join(cfg.DataDir, "secrets.key")
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return fmt.Errorf("init: rand for master key: %w", err)
		}
		if err := os.WriteFile(keyPath, []byte(base64.StdEncoding.EncodeToString(key)+"\n"), 0o600); err != nil {
			return fmt.Errorf("init: write master key: %w", err)
		}
		fmt.Printf("created  %s (mode 0600)\n", keyPath)
	}

	// 4. Local admin user.
	adminPwd := genTokenString(20) // human-typeable-ish
	pwHash, err := password.HashPassword(adminPwd)
	if err != nil {
		return err
	}
	admin := types.Subject{
		ID:       "local-admin",
		Name:     "admin",
		Provider: "local",
		Metadata: map[string]string{"secret_hash": pwHash},
	}
	if err := st.PutSubject(ctx, admin); err != nil {
		return err
	}
	fmt.Printf("created  admin user 'admin' (provider=local; password printed below — STORE IT)\n")
	fmt.Printf("   admin password: %s\n", adminPwd)

	// 5. Bootstrap token.
	token := genTokenString(32)
	tokHash, err := password.HashPassword(token)
	if err != nil {
		return err
	}
	bootstrap := types.Subject{
		ID:       "bootstrap-admin",
		Name:     "bootstrap",
		Provider: "bootstrap",
		Metadata: map[string]string{"secret_hash": tokHash},
	}
	if err := st.PutSubject(ctx, bootstrap); err != nil {
		return err
	}
	tokPath := filepath.Join(cfg.DataDir, "token")
	if err := os.WriteFile(tokPath, []byte(token+"\n"), 0o600); err != nil {
		return fmt.Errorf("init: write token file: %w", err)
	}
	fmt.Printf("created  bootstrap token (saved to %s, mode 0600)\n", tokPath)
	fmt.Printf("   bootstrap token: %s\n", token)

	// 6. Bind admin policy so the bootstrap subject can do everything.
	policy := types.Policy{
		ID:        "policy-bootstrap-admin",
		SubjectID: "bootstrap-admin",
		Role:      types.RoleAdmin,
		Project:   "*",
		CreatedAt: time.Now().UTC(),
	}
	if err := st.PutPolicy(ctx, policy); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("Run `proxa server` in a separate terminal (or under systemd) to start the")
	fmt.Println("control plane, then use `proxa up <file>` from any other terminal.")
	return nil
}

func genTokenString(n int) string {
	buf := make([]byte, n)
	_, _ = rand.Read(buf)
	return base64.RawURLEncoding.EncodeToString(buf)
}
