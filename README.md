# profuse

A read-only Proton Drive FUSE client for Linux. Mounts your Proton Drive as a
local filesystem using the OS keyring (libsecret) for secure, unattended
startup.

## Requirements

- `fuse3`
- `libsecret` (plus a running Secret Service daemon — GNOME Keyring or KWallet)
- A Proton account

## Installation

### From AUR

```bash
yay -S profuse-git
```

### From source

```bash
git clone https://github.com/nick/profuse
cd profuse
go build -o profuse ./cmd/
install -Dm755 profuse /usr/local/bin/profuse
```

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
fusermount3 -u ~/ProtonDrive
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

### First submission

```bash
# 1. Validate the build locally
makepkg -si
namcap PKGBUILD
namcap profuse-git-*.pkg.tar.zst

# 2. Generate the required metadata file
makepkg --printsrcinfo > .SRCINFO

# 3. Push to AUR (creating the package if it doesn't exist)
git remote add aur ssh://aur@aur.archlinux.org/profuse-git.git
git add .SRCINFO
git commit -m "Add .SRCINFO"
git push aur main
```

### Updating the package

Every time PKGBUILD changes (new pkgrel, updated deps, etc.):

```bash
makepkg --printsrcinfo > .SRCINFO
git add PKGBUILD .SRCINFO
git commit -m "Update to r2.abc1234"
git push origin main   # GitHub
git push aur main      # AUR
```

### Common namcap warnings

- **`SKIP` checksum** — expected for `-git` packages, not an error.
- **Missing license file** — add a `LICENSE` file to the repo; namcap expects
  it to be installed into the package under `/usr/share/licenses/profuse/`.
  Add to PKGBUILD:
  ```bash
  install -Dm644 LICENSE "$pkgdir/usr/share/licenses/$pkgname/LICENSE"
  ```
- **ELF file outside allowed dirs** — make sure the binary goes to
  `/usr/bin/`, not `/usr/local/bin/`.
- **Dependency not listed** — if namcap flags a linked library, add the
  package that owns it to `depends`.

### SSH keys

AUR uses a separate SSH remote but you can reuse your existing GitHub public
key — just add it to your AUR account at
https://aur.archlinux.org/account (Settings → SSH Keys).

Both remotes can coexist:

```bash
git remote add origin git@github.com:nick/profuse.git
git remote add aur    ssh://aur@aur.archlinux.org/profuse-git.git
```
