package fs

import (
	"context"
	"encoding/base64"
	"fmt"
	"syscall"

	proton "github.com/ProtonMail/go-proton-api"
	pgpcrypto "github.com/ProtonMail/gopenpgp/v2/crypto"
	gofuse "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// FileNode is a FUSE inode representing a Proton Drive file.
type FileNode struct {
	gofuse.Inode

	driveFS *DriveFS
	link    proton.Link
	nodeKR  *pgpcrypto.KeyRing // unlocked key ring for this file's node
}

var _ gofuse.NodeGetattrer = (*FileNode)(nil)
var _ gofuse.NodeOpener = (*FileNode)(nil)

// Getattr returns file metadata.
func (f *FileNode) Getattr(_ context.Context, _ gofuse.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = syscall.S_IFREG | 0o444
	out.Size = uint64(f.link.Size)
	out.Mtime = uint64(f.link.ModifyTime)
	out.Ctime = uint64(f.link.ModifyTime)
	out.Atime = uint64(f.link.ModifyTime)
	return 0
}

// Open returns a FileHandle that decrypts the file's blocks on read.
//
// The session key is derived here (once per open) by decrypting the
// ContentKeyPacket with the node key ring.
func (f *FileNode) Open(_ context.Context, flags uint32) (gofuse.FileHandle, uint32, syscall.Errno) {
	if flags&(syscall.O_WRONLY|syscall.O_RDWR|syscall.O_TRUNC) != 0 {
		return nil, 0, syscall.EROFS
	}

	keyPacket, err := base64.StdEncoding.DecodeString(f.link.FileProperties.ContentKeyPacket)
	if err != nil {
		return nil, 0, errno(fmt.Errorf("decoding content key packet: %w", err))
	}

	sessionKey, err := f.nodeKR.DecryptSessionKey(keyPacket)
	if err != nil {
		return nil, 0, errno(fmt.Errorf("decrypting session key: %w", err))
	}

	return &FileHandle{
		driveFS:    f.driveFS,
		link:       f.link,
		sessionKey: sessionKey,
	}, fuse.FOPEN_KEEP_CACHE, 0
}
