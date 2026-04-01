package fs

import (
	"context"
	"sync"
	"syscall"
	"time"

	proton "github.com/ProtonMail/go-proton-api"
	pgpcrypto "github.com/ProtonMail/gopenpgp/v2/crypto"
	gofuse "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// DirNode is a FUSE inode representing a Proton Drive folder.
//
// Children are loaded lazily on the first Readdir or Lookup call, then cached
// with a short TTL to reduce redundant API round-trips.
type DirNode struct {
	gofuse.Inode

	driveFS *DriveFS
	link    proton.Link
	nodeKR  *pgpcrypto.KeyRing // unlocked key ring for THIS directory

	mu         sync.Mutex
	children   []childEntry
	childrenAt time.Time
}

const childrenTTL = 5 * time.Second

type childEntry struct {
	link   proton.Link
	name   string
	nodeKR *pgpcrypto.KeyRing
}

var _ gofuse.NodeGetattrer = (*DirNode)(nil)
var _ gofuse.NodeLookuper = (*DirNode)(nil)
var _ gofuse.NodeReaddirer = (*DirNode)(nil)

// Getattr returns directory metadata.
func (d *DirNode) Getattr(_ context.Context, _ gofuse.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = syscall.S_IFDIR | 0o555
	out.Mtime = uint64(d.link.ModifyTime)
	out.Ctime = uint64(d.link.ModifyTime)
	out.Atime = uint64(d.link.ModifyTime)
	return 0
}

// Readdir lists the directory contents.
func (d *DirNode) Readdir(ctx context.Context) (gofuse.DirStream, syscall.Errno) {
	entries, err := d.loadChildren(ctx)
	if err != nil {
		return nil, errno(err)
	}

	result := make([]fuse.DirEntry, 0, len(entries))
	for _, e := range entries {
		mode := uint32(syscall.S_IFREG | 0o444)
		if e.link.Type == proton.FolderLinkType {
			mode = syscall.S_IFDIR | 0o555
		}
		result = append(result, fuse.DirEntry{
			Name: e.name,
			Mode: mode,
		})
	}

	return gofuse.NewListDirStream(result), 0
}

// Lookup finds a child inode by name.
func (d *DirNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*gofuse.Inode, syscall.Errno) {
	entries, err := d.loadChildren(ctx)
	if err != nil {
		return nil, errno(err)
	}

	for _, e := range entries {
		if e.name != name {
			continue
		}

		out.Mtime = uint64(e.link.ModifyTime)
		out.Ctime = uint64(e.link.ModifyTime)

		if e.link.Type == proton.FolderLinkType {
			out.Mode = syscall.S_IFDIR | 0o555
			child := &DirNode{driveFS: d.driveFS, link: e.link, nodeKR: e.nodeKR}
			return d.NewInode(ctx, child, gofuse.StableAttr{
				Mode: syscall.S_IFDIR,
				Ino:  linkIno(e.link.LinkID),
			}), 0
		}

		out.Mode = syscall.S_IFREG | 0o444
		out.Size = uint64(e.link.Size)
		child := &FileNode{driveFS: d.driveFS, link: e.link, nodeKR: e.nodeKR}
		return d.NewInode(ctx, child, gofuse.StableAttr{
			Mode: syscall.S_IFREG,
			Ino:  linkIno(e.link.LinkID),
		}), 0
	}

	return nil, syscall.ENOENT
}

// loadChildren fetches and decrypts this directory's children, using a
// short-lived cache to avoid hammering the API on rapid repeated lookups.
func (d *DirNode) loadChildren(ctx context.Context) ([]childEntry, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if time.Since(d.childrenAt) < childrenTTL {
		return d.children, nil
	}

	links, err := d.driveFS.client.ListChildren(ctx, d.driveFS.shareID, d.link.LinkID)
	if err != nil {
		return nil, err
	}

	entries := make([]childEntry, 0, len(links))
	for _, l := range links {
		if l.State != proton.ActiveLinkState {
			continue
		}

		name, err := decryptName(l, d.nodeKR)
		if err != nil {
			// Skip entries whose names can't be decrypted (graceful degradation).
			continue
		}

		childKR, err := l.GetKeyRing(d.nodeKR)
		if err != nil {
			continue
		}

		entries = append(entries, childEntry{link: l, name: name, nodeKR: childKR})
	}

	d.children = entries
	d.childrenAt = time.Now()
	return entries, nil
}

// decryptName decrypts a link's PGP-armored name using the parent's key ring.
func decryptName(l proton.Link, parentKR *pgpcrypto.KeyRing) (string, error) {
	encName, err := pgpcrypto.NewPGPMessageFromArmored(l.Name)
	if err != nil {
		return "", err
	}
	plain, err := parentKR.Decrypt(encName, nil, pgpcrypto.GetUnixTime())
	if err != nil {
		return "", err
	}
	return plain.GetString(), nil
}

// linkIno converts a link ID to a stable uint64 inode number via FNV-1a.
func linkIno(id string) uint64 {
	var h uint64 = 14695981039346656037
	for i := 0; i < len(id); i++ {
		h ^= uint64(id[i])
		h *= 1099511628211
	}
	return h
}
