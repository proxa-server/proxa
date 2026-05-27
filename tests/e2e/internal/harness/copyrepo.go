package harness

import (
	"io/fs"
	"os"
	"path/filepath"
)

// CopyRepoForTest copies src to dst, skipping .git/, bin/, and dist/
// (they bloat the copy without affecting tool/binary tests). Used by
// fresh-clone smoke tests like tool_directive_test.go.
func CopyRepoForTest(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch rel {
			case ".git", "bin", "dist":
				return filepath.SkipDir
			}
			if rel == "." {
				return nil
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), data, 0o644)
	})
}
