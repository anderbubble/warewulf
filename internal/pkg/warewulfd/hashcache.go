package warewulfd

import (
	"os"
	"sync"
	"syscall"

	"github.com/warewulf/warewulf/internal/pkg/util"
)

const maxDigestCacheEntries = 512

// fileDigests caches SHA-256 digests of files sent to TPM nodes, so a boot
// storm hashes each image once rather than once per node.
var fileDigests = newDigestCache(util.HashFile)

type fileKey struct {
	dev, ino    uint64
	size, mtime int64
}

type digestCall struct {
	done   chan struct{}
	digest string
	err    error
}

// digestCache maps a file's identity and version (device, inode, size and
// modification time) to its digest. Concurrent requests for the same file
// share one hash computation.
type digestCache struct {
	hash func(*os.File) (string, error)

	mu       sync.Mutex
	entries  map[fileKey]string
	order    []fileKey
	inflight map[fileKey]*digestCall
}

func newDigestCache(hash func(*os.File) (string, error)) *digestCache {
	return &digestCache{
		hash:     hash,
		entries:  map[fileKey]string{},
		inflight: map[fileKey]*digestCall{},
	}
}

// sum returns the hex SHA-256 of f's contents. f is left at offset 0.
func (c *digestCache) sum(f *os.File) (string, error) {
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	key := fileKey{size: info.Size(), mtime: info.ModTime().UnixNano()}
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		key.dev, key.ino = uint64(st.Dev), st.Ino
	}

	c.mu.Lock()
	if digest, ok := c.entries[key]; ok {
		c.mu.Unlock()
		return digest, nil
	}
	if call, ok := c.inflight[key]; ok {
		c.mu.Unlock()
		<-call.done
		return call.digest, call.err
	}
	call := &digestCall{done: make(chan struct{})}
	c.inflight[key] = call
	c.mu.Unlock()

	call.digest, call.err = c.hash(f)

	c.mu.Lock()
	delete(c.inflight, key)
	if call.err == nil {
		c.entries[key] = call.digest
		c.order = append(c.order, key)
		if len(c.order) > maxDigestCacheEntries {
			delete(c.entries, c.order[0])
			c.order = c.order[1:]
		}
	}
	c.mu.Unlock()
	close(call.done)
	return call.digest, call.err
}

// sumPath is sum for a file by name.
func (c *digestCache) sumPath(name string) (string, error) {
	f, err := os.Open(name)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	return c.sum(f)
}
