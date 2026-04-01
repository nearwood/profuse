# TODO

## Correctness

- [ ] **Write support** — `Create`, `Write`, `Mkdir`, `Unlink`, `Rename`. The API calls exist (`CreateFile`, `CreateFolder`, `DeleteChildren`); mostly wiring them up and handling encryption (generating node keys, encrypting names, etc.)
- [ ] **Large file pagination** — `GetRevision` in v0.4.0 returns all blocks at once; files with hundreds of blocks may be slow to open or time out
- [ ] **Two-password mode** — currently returns an error; some older Proton accounts use it

## Reliability

- [ ] **Event polling** — poll `GetVolumeEvent` periodically and invalidate the directory cache so changes from other clients (web, mobile) show up without remounting
- [ ] **Block-level caching** — every read currently re-downloads the block even within the same session; a simple LRU cache in `FileHandle` keyed by `(linkID, blockIndex)` would make repeated reads fast
- [ ] **Retry/backoff** — network errors bubble up as `EIO`; wrapping API calls with exponential backoff would make the mount more resilient to transient failures

## Packaging

- [ ] **Add a `LICENSE` file** — namcap will flag its absence when run against the built package
- [ ] **Version flag** — wire up `profuse --version` using `-ldflags "-X main.version=..."`; the PKGBUILD already passes this, the binary just doesn't use it yet
- [ ] **`.SRCINFO` and AUR submission** — when happy with the feature set

## Stretch

- [ ] **Trash support** — `profuse trash` to list/restore trashed files
- [ ] **Shared drives** — currently only mounts the primary share; shared folders from other users are separate shares
- [ ] **Photo/album support** — Proton Drive has album links that aren't currently handled
