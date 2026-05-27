# Contract — `install.sh`

POSIX-shell installer for the Proxa control-plane binary. Tested with `dash`, `bash`, and `zsh`.

## Invocation

```sh
curl -fsSL https://proxa-server.github.io/proxa/install/install.sh | sh
```

Override defaults via env vars:

```sh
curl -fsSL https://proxa-server.github.io/proxa/install/install.sh \
  | INSTALL_VERSION=v0.4.2 INSTALL_DIR=$HOME/.local/bin sh
```

## Environment variables

| Var | Default | Meaning |
|---|---|---|
| `INSTALL_VERSION` | `latest` | Tag to install. `latest` resolves via GitHub API redirect to most recent release. |
| `INSTALL_DIR` | `/usr/local/bin` | Target directory. Must be writable (or installer will use `sudo` interactively). |
| `INSTALL_VERIFY` | `1` | Set to `0` to skip checksum verification (NOT RECOMMENDED). |

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Success: binary installed, version verified, systemd unit text printed |
| 1 | Unsupported platform (Alpine musl, FreeBSD, Windows, or unrecognized arch) |
| 2 | Download failed (network, 404, missing release artifact) |
| 3 | Checksum mismatch (downloaded file does not match `checksums.txt`) |
| 4 | Install directory not writable (and `sudo` unavailable or refused) |
| 5 | Missing `curl` AND `wget` (one is required) |

## Platform support matrix (v0.4.2)

| OS | Arch | Support |
|---|---|---|
| Linux glibc (Ubuntu, Debian, RHEL, Fedora) | amd64, arm64 | ✅ supported |
| Linux musl (Alpine) | any | ❌ refuse with msg + manual download link |
| macOS | amd64, arm64 | ⚠️ binary downloaded (for local dev), suggested launchd plist printed instead of systemd |
| FreeBSD | any | ❌ refuse with msg + "build from source" link |
| Windows | any | ❌ refuse with msg + "download .exe from releases" link |

## Stdout format

One event per line. Machine-parseable (lines starting with `>>>` are status events, plain lines are operator-visible messages).

```
>>> detecting platform
>>> detected: linux/amd64
>>> resolving latest version
>>> resolved version: v0.4.2
>>> downloading proxa_v0.4.2_linux_amd64.tar.gz
>>> downloading checksums.txt
>>> verifying SHA-256 checksum
>>> verified: 5f3e... matches
>>> extracting archive
>>> installing /usr/local/bin/proxa (mode 0755)
>>> verifying installed binary
>>> installed: proxa v0.4.2 (commit 56f3673, built 2026-05-21T04:29:07Z)
>>> done

Suggested systemd unit (copy to /etc/systemd/system/proxa.service):

  [Unit]
  Description=Proxa orchestrator
  After=network.target

  [Service]
  Type=simple
  ExecStart=/usr/local/bin/proxa server
  Restart=on-failure
  User=proxa
  Environment=PROXA_DATA_DIR=/var/lib/proxa

  [Install]
  WantedBy=multi-user.target

Then: systemctl enable --now proxa
```

## Stderr format

Errors only. Format: `error: <human message>`, optional `hint: <suggestion>` line follows.

```
error: checksum mismatch for proxa_v0.4.2_linux_amd64.tar.gz
hint: re-run with INSTALL_VERIFY=0 to skip (NOT recommended) or re-download manually
```

## Behavior invariants

1. **Atomicity**: temp file used for download + verify; only moved to `INSTALL_DIR` after checksum passes. A partial / corrupted download NEVER overwrites an existing binary.
2. **Idempotent**: re-running the installer with the same version is a no-op if the existing binary's checksum matches. Re-running with a newer version replaces atomically.
3. **No daemon side-effects**: installer does NOT start any service, write to `/etc/systemd/`, or modify firewall. Operator does that after install (printed unit text).
4. **Verify by default**: `INSTALL_VERIFY=0` is opt-out; the default path enforces SHA-256 match.
5. **Tool fallback**: prefers `curl`, falls back to `wget`. Errors clearly if neither exists (exit 5).

## Test coverage

`tests/e2e/install_sh_test.go` (tagged `//go:build e2e`):
- TestInstallSh_HappyPath_LinuxAmd64 — spin up `debian:12` container, `apt-get install -y curl`, run install.sh, verify `/usr/local/bin/proxa` exists and `proxa version` returns expected.
- TestInstallSh_RefuseAlpine — spin up `alpine:3.20` container, run install.sh, assert exit 1 + error message mentions "Alpine musl".
- TestInstallSh_RefuseChecksumMismatch — locally serve a tampered `checksums.txt` via httptest, override `INSTALL_BASE_URL` to point at it, assert exit 3.
- TestInstallSh_NoCurlNoWget — spin up minimal busybox container with neither curl nor wget, run via sh-piped-stdin, assert exit 5.
- TestInstallSh_Idempotent — install v0.4.1, then v0.4.2, verify version reports v0.4.2 + no orphan files.

Skipped on hosts where Docker is unavailable.
