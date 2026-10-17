package tpm

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	warewulfconf "github.com/warewulf/warewulf/internal/pkg/config"
)

// Store keeps one State file per node, readable only by root. Updates are
// serialized per node within a process and across processes (warewulfd and
// wwctl), and written atomically.
type Store struct {
	dir string

	mu    sync.Mutex
	nodes map[string]*sync.Mutex
}

// NewStore returns a Store in dir.
func NewStore(dir string) *Store {
	return &Store{dir: dir, nodes: map[string]*sync.Mutex{}}
}

var (
	defaultStore     *Store
	defaultStoreOnce sync.Once
)

// DefaultStore returns the Store in the configured TPM state directory.
func DefaultStore() *Store {
	defaultStoreOnce.Do(func() {
		conf := warewulfconf.Get()
		defaultStore = NewStore(conf.Paths.TPMStatedir())
	})
	return defaultStore
}

// ResetDefaultStore makes the next DefaultStore call re-read the configured
// paths. For tests that change the configuration.
func ResetDefaultStore() {
	defaultStoreOnce = sync.Once{}
}

func validNode(node string) error {
	if node == "" || node == "." || node == ".." || strings.ContainsAny(node, "/\x00") {
		return fmt.Errorf("invalid node name %q", node)
	}
	return nil
}

// Path returns the state file for node.
func (s *Store) Path(node string) string {
	return filepath.Join(s.dir, node+".json")
}

// Exists reports whether node has any TPM state.
func (s *Store) Exists(node string) bool {
	_, err := os.Stat(s.Path(node))
	return err == nil
}

// Load returns node's state, or an empty State if it has none. It does not
// lock: writes are atomic renames, so a read sees a complete file.
func (s *Store) Load(node string) (*State, error) {
	if err := validNode(node); err != nil {
		return nil, err
	}
	st, err := s.read(node)
	if err != nil {
		return nil, err
	}
	if st == nil {
		st = &State{Version: StateVersion, Node: node}
	}
	return st, nil
}

// Update loads node's state, applies fn, and saves the result. The state is
// saved even if fn returns an error, so fn can record a rejection; fn's error
// is returned.
func (s *Store) Update(node string, fn func(*State) error) error {
	if err := validNode(node); err != nil {
		return err
	}
	unlock, err := s.lock(node)
	if err != nil {
		return err
	}
	defer unlock()

	st, err := s.Load(node)
	if err != nil {
		return err
	}
	fnErr := fn(st)
	if err := s.write(node, st); err != nil {
		return err
	}
	return fnErr
}

// Delete removes node's state.
func (s *Store) Delete(node string) error {
	if err := validNode(node); err != nil {
		return err
	}
	unlock, err := s.lock(node)
	if err != nil {
		return err
	}
	defer unlock()
	// The lock file stays: removing it while holding its lock would let
	// another process lock a new file while this one still holds the old.
	if err := os.Remove(s.Path(node)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// Nodes returns the nodes that have state.
func (s *Store) Nodes() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var nodes []string
	for _, e := range entries {
		if name, ok := strings.CutSuffix(e.Name(), ".json"); ok && !e.IsDir() {
			nodes = append(nodes, name)
		}
	}
	return nodes, nil
}

// lock serializes updates to node within this process and across processes.
// The mutex covers this process: on NFS, Linux emulates flock with POSIX
// locks, which do not exclude other goroutines of the same process.
func (s *Store) lock(node string) (func(), error) {
	s.mu.Lock()
	m, ok := s.nodes[node]
	if !ok {
		m = &sync.Mutex{}
		s.nodes[node] = m
	}
	s.mu.Unlock()
	m.Lock()

	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		m.Unlock()
		return nil, err
	}
	f, err := os.OpenFile(s.Path(node)+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		m.Unlock()
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		m.Unlock()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
		m.Unlock()
	}, nil
}

func (s *Store) read(node string) (*State, error) {
	data, err := os.ReadFile(s.Path(node))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("reading TPM state for %s: %w", node, err)
	}
	if st.Version != StateVersion {
		return nil, fmt.Errorf("reading TPM state for %s: unsupported version %d", node, st.Version)
	}
	return &st, nil
}

func (s *Store) write(node string, st *State) error {
	st.Version = StateVersion
	st.Node = node
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, "."+node+".json.*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, s.Path(node))
}
