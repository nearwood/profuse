// Package fs implements a read-only FUSE filesystem backed by Proton Drive.
//
// Layout:
//
//	DriveFS  – shared state (client, share ID, address key ring)
//	DirNode  – FUSE directory, wraps a Proton Drive folder Link
//	FileNode – FUSE file, wraps a Proton Drive file Link
//	FileHandle – open file handle; streams + decrypts blocks on Read
package fs

import (
	"context"
	"fmt"
	"syscall"
	"time"

	proton "github.com/ProtonMail/go-proton-api"
	pgpcrypto "github.com/ProtonMail/gopenpgp/v2/crypto"
	gofuse "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// Options controls mount behaviour.
type Options struct {
	Debug    bool
	ReadOnly bool
}

// Mount mounts Proton Drive at mountpoint and blocks until the filesystem is
// unmounted (e.g. via `fusermount3 -u <mountpoint>`).
func Mount(ctx context.Context, mountpoint string, c *proton.Client, addrKR *pgpcrypto.KeyRing, opts Options) error {
	root, err := buildRoot(ctx, c, addrKR)
	if err != nil {
		return fmt.Errorf("building filesystem root: %w", err)
	}

	cacheTTL := 5 * time.Second
	mountOpts := fuse.MountOptions{
		FsName: "protondrive",
		Name:   "protondriveclient",
		Debug:  opts.Debug,
		// Suppress xattr calls; we don't implement them.
		DisableXAttrs: true,
	}
	if opts.ReadOnly {
		mountOpts.Options = append(mountOpts.Options, "ro")
	}

	fuseOpts := &gofuse.Options{
		MountOptions: mountOpts,
		EntryTimeout: &cacheTTL,
		AttrTimeout:  &cacheTTL,
	}

	server, err := gofuse.Mount(mountpoint, root, fuseOpts)
	if err != nil {
		return fmt.Errorf("mounting FUSE: %w", err)
	}

	fmt.Printf("Mounted. To unmount: fusermount3 -u %s\n", mountpoint)
	server.Wait()
	return nil
}

// buildRoot finds the primary share and builds the root DirNode.
func buildRoot(ctx context.Context, c *proton.Client, addrKR *pgpcrypto.KeyRing) (*DirNode, error) {
	// List shares and pick the primary one (user's own Drive root).
	shares, err := c.ListShares(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("listing shares: %w", err)
	}

	var mainShare proton.Share
	for _, s := range shares {
		if s.Flags == proton.PrimaryShare {
			mainShare = s
			break
		}
	}
	if mainShare.ShareID == "" {
		return nil, fmt.Errorf("no primary share found")
	}

	// Fetch full share details (includes Key + Passphrase for decryption).
	share, err := c.GetShare(ctx, mainShare.ShareID)
	if err != nil {
		return nil, fmt.Errorf("getting share %s: %w", mainShare.ShareID, err)
	}

	shareKR, err := share.GetKeyRing(addrKR)
	if err != nil {
		return nil, fmt.Errorf("unlocking share key ring: %w", err)
	}

	// The share's root link is the filesystem root.
	rootLink, err := c.GetLink(ctx, share.ShareID, share.LinkID)
	if err != nil {
		return nil, fmt.Errorf("getting root link: %w", err)
	}

	rootKR, err := rootLink.GetKeyRing(shareKR)
	if err != nil {
		return nil, fmt.Errorf("unlocking root link key ring: %w", err)
	}

	driveFS := &DriveFS{
		client:  c,
		addrKR:  addrKR,
		shareID: share.ShareID,
	}

	return &DirNode{
		driveFS: driveFS,
		link:    rootLink,
		nodeKR:  rootKR,
	}, nil
}

// DriveFS holds shared state for the whole mounted filesystem.
type DriveFS struct {
	client  *proton.Client
	addrKR  *pgpcrypto.KeyRing
	shareID string
}

// errno maps errors to FUSE errno values.
func errno(err error) syscall.Errno {
	if err == nil {
		return 0
	}
	return syscall.EIO
}
