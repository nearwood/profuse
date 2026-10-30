# profuse

![Go Version](https://img.shields.io/badge/Go-1.26%2B-00ADD8.svg) ![FUSE](https://img.shields.io/badge/FUSE-3-orange.svg) ![License](https://img.shields.io/badge/License-GPLv3-green.svg) [![AUR](https://img.shields.io/aur/version/profuse)](https://aur.archlinux.org/packages/profuse) [![AUR git](https://img.shields.io/aur/version/profuse-git?label=aur-git)](https://aur.archlinux.org/packages/profuse-git)

> ⚠️ **Unofficial** - Not affiliated with or endorsed by Proton AG or Proton Drive. ⚠️

A read-only Proton Drive FUSE client for Linux. Mounts your Proton Drive as a
local filesystem using the OS keyring (libsecret) for secure, unattended
startup.

## Requirements

- `fuse3`
- `libsecret` (plus a running Secret Service daemon — GNOME Keyring or KWallet)
- A Proton account

## General Idea

This installs a binary and user service. You use the binary to login, the user service then handles mounting automatically at login.

## Installation

### From AUR

Two packages are available:

- `profuse` — the latest tagged release
- `profuse-git` — built from the latest commit on `main`

```bash
yay -S profuse      # or profuse-git
```

Or build from this repo: `cd contrib/aur/profuse && makepkg -si`

## Usage

### First-time setup

```bash
# Authenticate — stores session tokens on disk and key password in keyring
profuse auth login

# Create a mountpoint and mount
mkdir -p ~/ProtonDrive
profuse mount ~/ProtonDrive
```

To unmount:

```bash
profuse unmount ~/ProtonDrive
```

### Auth commands

```bash
profuse auth login     # Authenticate (prompts for username, password, 2FA if needed)
profuse auth logout    # Revoke session and remove all stored credentials
profuse auth status    # Show currently logged-in username
```

### Running in the background (systemd)

Install the user service:

```bash
# If installed from AUR the service file is already in place; otherwise:
install -Dm644 contrib/systemd/profuse.service ~/.config/systemd/user/profuse.service

systemctl --user enable --now profuse
```

The service starts at login, reads the key password silently from the keyring,
and mounts at `~/ProtonDrive`. Logs are available via:

```bash
journalctl --user -u profuse -f
```

## Stored credentials

| What | Where |
|---|---|
| Session tokens | `~/.config/profuse/session.json` |
| Key password | OS keyring (`profuse` service, libsecret) |

Neither file contains your plaintext password.

---

## AUR Maintenance

The PKGBUILDs live in `contrib/aur/profuse/` (releases) and
`contrib/aur/profuse-git/` (latest commit).

### Versioning

The version lives only in git tags (`vX.Y.Z`); nothing in the repo needs
editing to bump it. Before 1.0, bump the patch number (`0.1.2`) for fixes and
the minor number (`0.2.0`) for features or breaking changes.

From the tag, the release workflow derives:

| Where | Version |
|---|---|
| `profuse --version` | `0.2.0` (via `-X main.version`) |
| `profuse` package | `pkgver=0.2.0`, `pkgrel=1` |
| `profuse-git` package | `0.2.0.r0.g<hash>` when published; local builds compute `0.2.0.r<commits since tag>.g<hash>` |

The `pkgver` and `sha256sums` in the repo copy of `contrib/aur/profuse/PKGBUILD`
are placeholders — the workflow fills in the real values when publishing.

### Releasing

```bash
git tag -a v0.2.0 -m v0.2.0
git push origin v0.2.0
gh release create v0.2.0 --generate-notes   # or publish it in the GitHub UI
```

Publishing the release (not a draft) runs `.github/workflows/aur-publish.yml`,
which sets the versions above, fills in the source checksum, regenerates
`.SRCINFO`, and pushes both packages to the AUR. It needs the
`AUR_SSH_PRIVATE_KEY` secret (a passphrase-less private key) and the
`AUR_USERNAME` / `AUR_EMAIL` variables.

### Packaging-only changes

For a PKGBUILD fix with no code change (a missing dependency, a build flag),
bump `pkgrel` instead of the version:

1. Edit `contrib/aur/<pkgname>/PKGBUILD`, check it locally, and commit to `main`:

   ```bash
   cd contrib/aur/profuse      # or profuse-git
   makepkg -si
   namcap PKGBUILD
   namcap profuse-*.pkg.tar.zst
   ```

2. Run the workflow manually: **Actions → AUR publish → Run workflow** on
   `main`, with the current release tag, the next `pkgrel` (e.g. `2`), and the
   package(s) that changed. Or:

   ```bash
   gh workflow run aur-publish.yml -f tag=v0.2.0 -f pkgrel=2 -f package=profuse
   ```

Manual runs take the PKGBUILDs from the branch they run on and the version
from the tag. The next release resets `pkgrel` to 1. The same manual run with
`pkgrel=1` also re-publishes a release whose workflow run failed.

### Common namcap warnings

- **`SKIP` checksum** — expected for `-git` packages, and for the repo copy
  of `profuse` (the release workflow fills in the real checksum).
- **ELF file outside allowed dirs** — make sure the binary goes to
  `/usr/bin/`, not `/usr/local/bin/`.
- **Dependency not listed** — if namcap flags a linked library, add the
  package that owns it to `depends`.
