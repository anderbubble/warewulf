package warewulfd

import (
	"crypto/rand"
	"crypto/subtle"
	"sync"
	"time"
)

const (
	nonceTTL         = 60 * time.Second
	nonceSize        = 32
	maxNoncesPerNode = 4
)

// nonceStore holds the nonces issued for TPM quotes. Each is single-use and
// short-lived, so a quote cannot be replayed. Nonces are kept in memory: after
// a restart, a node's next quote is rejected and it fetches a new nonce. They
// are issued only to configured TPM nodes, a few each, which bounds the store.
type nonceStore struct {
	mu     sync.Mutex
	byNode map[string][]issuedNonce
}

type issuedNonce struct {
	value   []byte
	expires time.Time
}

func newNonceStore() *nonceStore {
	return &nonceStore{byNode: map[string][]issuedNonce{}}
}

// issue returns a new nonce for node. Each node may hold a few at once; the
// oldest is dropped beyond that.
func (s *nonceStore) issue(node string, now time.Time) ([]byte, error) {
	value := make([]byte, nonceSize)
	if _, err := rand.Read(value); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	list := s.unexpired(node, now)
	if len(list) >= maxNoncesPerNode {
		list = list[1:]
	}
	s.byNode[node] = append(list, issuedNonce{value: value, expires: now.Add(nonceTTL)})
	return value, nil
}

// consume reports whether nonce was issued to node and is unexpired, and
// removes it.
func (s *nonceStore) consume(node string, nonce []byte, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := s.unexpired(node, now)
	for i, n := range list {
		if subtle.ConstantTimeCompare(n.value, nonce) == 1 {
			s.byNode[node] = append(list[:i:i], list[i+1:]...)
			return true
		}
	}
	s.byNode[node] = list
	return false
}

// unexpired returns node's unexpired nonces.
func (s *nonceStore) unexpired(node string, now time.Time) []issuedNonce {
	var kept []issuedNonce
	for _, n := range s.byNode[node] {
		if now.Before(n.expires) {
			kept = append(kept, n)
		}
	}
	return kept
}
