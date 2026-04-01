package fs

import (
	"context"
	"fmt"
	"io"
	"syscall"

	proton "github.com/ProtonMail/go-proton-api"
	pgpcrypto "github.com/ProtonMail/gopenpgp/v2/crypto"
	gofuse "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// blockSize is Proton's fixed block size (4 MiB).
const blockSize = 4 * 1024 * 1024

// FileHandle implements Read for an open Proton Drive file.
//
// The kernel page cache is used for caching (FOPEN_KEEP_CACHE from Open), so
// each block is typically fetched only once per mount per file.
type FileHandle struct {
	driveFS    *DriveFS
	link       proton.Link
	sessionKey *pgpcrypto.SessionKey

	// Lazily populated on first Read.
	blocks     []proton.Block
	blocksErr  error
	blocksDone bool
}

var _ gofuse.FileReader = (*FileHandle)(nil)

// Read satisfies FUSE read requests.  The kernel may issue multiple calls for
// a single application read, each with a specific offset and buffer size.
func (fh *FileHandle) Read(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	if err := fh.ensureBlocks(ctx); err != nil {
		return nil, errno(err)
	}

	if len(fh.blocks) == 0 || off >= fh.link.Size {
		return fuse.ReadResultData(nil), 0
	}

	end := off + int64(len(dest))
	if end > fh.link.Size {
		end = fh.link.Size
	}

	firstBlock := int(off / blockSize)
	lastBlock := int((end - 1) / blockSize)

	var out []byte
	for idx := firstBlock; idx <= lastBlock && idx < len(fh.blocks); idx++ {
		plain, err := fh.fetchAndDecrypt(ctx, fh.blocks[idx])
		if err != nil {
			return nil, errno(err)
		}

		// Slice the portion of this block that falls within [off, end).
		blockStart := int64(idx) * blockSize
		lo := off - blockStart
		if lo < 0 {
			lo = 0
		}
		hi := end - blockStart
		if hi > int64(len(plain)) {
			hi = int64(len(plain))
		}
		out = append(out, plain[lo:hi]...)
	}

	return fuse.ReadResultData(out), 0
}

// ensureBlocks fetches the block list from the active revision once and caches it.
func (fh *FileHandle) ensureBlocks(ctx context.Context) error {
	if fh.blocksDone {
		return fh.blocksErr
	}
	fh.blocksDone = true

	revID := fh.link.FileProperties.ActiveRevision.ID
	if revID == "" {
		fh.blocksErr = fmt.Errorf("no active revision for link %s", fh.link.LinkID)
		return fh.blocksErr
	}

	rev, err := fh.driveFS.client.GetRevision(
		ctx,
		fh.driveFS.shareID,
		fh.link.LinkID,
		revID,
	)
	if err != nil {
		fh.blocksErr = fmt.Errorf("getting revision: %w", err)
		return fh.blocksErr
	}

	fh.blocks = rev.Blocks
	return nil
}

// fetchAndDecrypt downloads a block and decrypts it with the file's session key.
func (fh *FileHandle) fetchAndDecrypt(ctx context.Context, block proton.Block) ([]byte, error) {
	rc, err := fh.driveFS.client.GetBlock(ctx, block.URL)
	if err != nil {
		return nil, fmt.Errorf("downloading block %d: %w", block.Index, err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("reading block %d: %w", block.Index, err)
	}

	plain, err := fh.sessionKey.Decrypt(data)
	if err != nil {
		return nil, fmt.Errorf("decrypting block %d: %w", block.Index, err)
	}

	return plain.GetBinary(), nil
}
