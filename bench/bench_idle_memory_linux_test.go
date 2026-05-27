//go:build linux

package bench_test

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// BenchmarkIdleMemoryRSS_Linux spawns `proxa server` as a subprocess
// on a fresh data dir, lets it settle for 10s, reads VmRSS from
// /proc/<pid>/status, then kills the subprocess. Reports the
// committed-resident-set in MB as the "MB-rss" metric.
//
// Linux-only: macOS and Windows do not expose VmRSS via procfs. The
// non-Linux portable fallback lives in bench_idle_memory_other_test.go.
//
// The benchmark loop runs `b.N` times but each iteration spawns +
// settles + samples + kills — so `make bench` invokes with -count=3
// to get 3 measurements without bloating runtime to N×N samples. The
// metric is the LATEST sample (b.N's value).
func BenchmarkIdleMemoryRSS_Linux(b *testing.B) {
	binary, err := findProxaBinary()
	if err != nil {
		b.Skipf("proxa binary not found: %v", err)
	}

	for i := 0; i < b.N; i++ {
		mb, err := sampleIdleRSSLinux(b, binary)
		if err != nil {
			b.Fatalf("sample %d: %v", i, err)
		}
		// Report the most-recent sample; older overwrites are fine —
		// the bench framework records one value per iteration.
		b.ReportMetric(mb, "MB-rss")
	}
}

func sampleIdleRSSLinux(b *testing.B, binary string) (float64, error) {
	dataDir := b.TempDir()

	// proxa init creates the SQLite DB + bootstrap token.
	init := exec.Command(binary, "init")
	init.Env = append(os.Environ(), "PROXA_DATA_DIR="+dataDir)
	if out, err := init.CombinedOutput(); err != nil {
		return 0, fmt.Errorf("proxa init: %w\n%s", err, out)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "server")
	cmd.Env = append(os.Environ(), "PROXA_DATA_DIR="+dataDir)
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start proxa server: %w", err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	// 10s settle so the runtime stops allocating during startup.
	time.Sleep(10 * time.Second)

	return readVmRSSMB(cmd.Process.Pid)
}

func readVmRSSMB(pid int) (float64, error) {
	f, err := os.Open(filepath.Join("/proc", strconv.Itoa(pid), "status"))
	if err != nil {
		return 0, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "VmRSS:") {
			continue
		}
		// Format: "VmRSS:\t   12345 kB"
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, fmt.Errorf("unexpected VmRSS line: %q", line)
		}
		kb, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			return 0, fmt.Errorf("parse VmRSS value %q: %w", fields[1], err)
		}
		return kb / 1024.0, nil
	}
	return 0, fmt.Errorf("VmRSS line not found in /proc/%d/status", pid)
}

// findProxaBinary locates the proxa binary built by `make build`.
// Walks up from cwd looking for bin/proxa. Returns the absolute path.
func findProxaBinary() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, "bin", "proxa")
		if _, err := os.Stat(candidate); err == nil {
			return filepath.Abs(candidate)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
