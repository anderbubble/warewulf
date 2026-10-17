package tpm_test

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/warewulf/warewulf/internal/pkg/tpm"
	"github.com/warewulf/warewulf/internal/pkg/tpm/tpmtest"
)

func verified(t *testing.T, f *tpmtest.TPM, files []tpm.SentFile) (*tpm.Evidence, *tpm.Verified) {
	n := nonce()
	ev, err := f.Evidence(n, nil)
	require.NoError(t, err)
	v, err := tpm.VerifyEvidence(ev, n, files)
	require.NoError(t, err)
	return ev, v
}

// attest issues a secret for f's evidence as warewulfd does, and returns it
// with the evaluation error.
func attest(t *testing.T, st *tpm.State, f *tpmtest.TPM, files []tpm.SentFile, secret string) error {
	t.Helper()
	ev, v := verified(t, f, files)
	enroll, candidate, err := st.Evaluate(ev, v, time.Now())
	if candidate != nil {
		st.AddConfirmation([]byte(secret), candidate, time.Now())
	} else {
		st.AddPending([]byte(secret), ev, v, files, enroll, time.Now())
	}
	return err
}

func TestEnrollment(t *testing.T) {
	now := time.Now()
	f, files := booted(t)
	var st tpm.State

	require.NoError(t, attest(t, &st, f, files, "first"))
	assert.Nil(t, st.Enrolled, "nothing is enrolled before the node proves it holds the TPM")
	assert.Equal(t, "UNENROLLED", st.Status())
	require.True(t, st.CheckSecret([]byte("first"), now))
	require.NotNil(t, st.Enrolled, "using the secret enrolls")
	assert.Equal(t, "ENROLLED", st.Status())

	require.NoError(t, attest(t, &st, f, files, "same"), "same TPM and baseline is admitted")
	require.NoError(t, f.NewAK())
	require.NoError(t, attest(t, &st, f, files, "new AK"), "a new AK (re-provisioned node) is admitted")
	assert.True(t, st.CheckSecret([]byte("new AK"), now))

	// Two nodes race to enroll; the first to prove its TPM wins.
	var race tpm.State
	other, otherFiles := booted(t)
	require.NoError(t, attest(t, &race, f, files, "mine"))
	require.NoError(t, attest(t, &race, other, otherFiles, "theirs"))
	assert.True(t, race.CheckSecret([]byte("theirs"), now))
	assert.False(t, race.CheckSecret([]byte("mine"), now), "a different TPM can't use a secret issued before the other enrolled")
}

func TestCandidates(t *testing.T) {
	now := time.Now()
	f, files := booted(t)
	var st tpm.State
	require.NoError(t, attest(t, &st, f, files, "enroll"))
	require.True(t, st.CheckSecret([]byte("enroll"), now))

	f.Extend(0, 0x80000008, []byte("firmware update"))
	err := attest(t, &st, f, files, "changed")
	assert.Equal(t, tpm.CodeBaselineChanged, tpm.CodeOf(err))
	assert.Nil(t, st.Candidate, "a candidate is not recorded before the node proves it holds the TPM")
	assert.False(t, st.CheckSecret([]byte("changed"), now), "a candidate's secret doesn't unlock the runtime overlay")
	assert.Equal(t, "current", st.SecretStatus(), "a candidate's secret isn't a pending runtime secret")
	assert.True(t, st.CheckSecret([]byte("enroll"), now), "and doesn't displace the current one")

	_, ok := st.ConfirmCandidate([]byte("wrong"), now)
	assert.False(t, ok)
	c, ok := st.ConfirmCandidate([]byte("changed"), now)
	require.True(t, ok)
	assert.Equal(t, []int{0}, c.Changed)
	assert.Equal(t, "CANDIDATE", st.Status())
	_, ok = st.ConfirmCandidate([]byte("changed"), now)
	assert.False(t, ok, "a confirmation is single-use")

	require.NoError(t, st.Approve(now))
	assert.Nil(t, st.Candidate)
	assert.NoError(t, attest(t, &st, f, files, "approved"), "the approved baseline is admitted")
	assert.ErrorIs(t, st.Approve(now), tpm.ErrNoCandidate)

	other, otherFiles := booted(t)
	assert.Equal(t, tpm.CodeEKChanged, tpm.CodeOf(attest(t, &st, other, otherFiles, "other")), "a different TPM is not admitted")
	c, ok = st.ConfirmCandidate([]byte("other"), now)
	require.True(t, ok)
	assert.Equal(t, tpm.CodeEKChanged, c.Reason)

	// A proven attestation in the enrolled state clears a stale candidate.
	require.NoError(t, attest(t, &st, f, files, "back"))
	assert.True(t, st.CheckSecret([]byte("back"), now))
	assert.Nil(t, st.Candidate)
}

func TestConfirmationsDontEvictRuntimeSecrets(t *testing.T) {
	now := time.Now()
	f, files := booted(t)
	var st tpm.State
	require.NoError(t, attest(t, &st, f, files, "enroll"))
	require.True(t, st.CheckSecret([]byte("enroll"), now))
	require.NoError(t, attest(t, &st, f, files, "next"))

	changed, changedFiles := booted(t)
	for i := range tpm.MaxPending + 2 {
		err := attest(t, &st, changed, changedFiles, fmt.Sprintf("change %d", i))
		require.Equal(t, tpm.CodeEKChanged, tpm.CodeOf(err))
	}
	assert.Len(t, st.Confirmations, tpm.MaxPending)
	assert.True(t, st.CheckSecret([]byte("next"), now), "the runtime secret is still pending")
	_, ok := st.ConfirmCandidate([]byte("change 0"), now)
	assert.False(t, ok, "the oldest confirmation is dropped beyond the limit")
	_, ok = st.ConfirmCandidate([]byte(fmt.Sprintf("change %d", tpm.MaxPending+1)), now)
	assert.True(t, ok)
}

func TestCandidateID(t *testing.T) {
	c := tpm.Candidate{Enrollment: tpm.Enrollment{
		EKFingerprint: "ek",
		Baseline:      map[int][]byte{0: {0}, 2: {2}, 4: {4}, 7: {7}},
	}}
	id := c.ID()
	assert.Len(t, id, 16)
	same := c
	same.Reason = tpm.CodeEKChanged
	assert.Equal(t, id, same.ID(), "the ID depends only on the TPM and baseline")

	other := c
	other.Baseline = map[int][]byte{0: {0}, 2: {2}, 4: {4}, 7: {8}}
	assert.NotEqual(t, id, other.ID(), "a different baseline has a different ID")
	other = c
	other.EKFingerprint = "other ek"
	assert.NotEqual(t, id, other.ID(), "a different TPM has a different ID")
}

func TestSecrets(t *testing.T) {
	now := time.Now()
	f, files := booted(t)
	ev, v := verified(t, f, files)
	var st tpm.State

	assert.False(t, st.CheckSecret([]byte("anything"), now))
	assert.False(t, st.CheckSecret(nil, now))

	st.AddPending([]byte("first"), ev, v, files, nil, now)
	assert.Equal(t, "pending(1)", st.SecretStatus())
	assert.True(t, st.CheckSecret([]byte("first"), now), "a pending secret is accepted")
	require.NotNil(t, st.Current)
	assert.Empty(t, st.Pending, "using a pending secret makes it current")

	// Someone else attests (or the node re-attests): the running secret
	// stays valid until the new one is used.
	st.AddPending([]byte("second"), ev, v, files, nil, now)
	assert.True(t, st.CheckSecret([]byte("first"), now), "current stays valid while another is pending")
	assert.True(t, st.CheckSecret([]byte("second"), now))
	assert.False(t, st.CheckSecret([]byte("first"), now), "replaced once the new secret is used")

	st.AddPending([]byte("stale"), ev, v, files, nil, now)
	assert.False(t, st.CheckSecret([]byte("stale"), now.Add(tpm.PendingTTL)), "pending secrets expire")

	for i := range tpm.MaxPending + 2 {
		st.AddPending([]byte{byte(i)}, ev, v, files, nil, now)
	}
	assert.Len(t, st.Pending, tpm.MaxPending)
	assert.False(t, st.CheckSecret([]byte{0}, now), "oldest pending secret is dropped")

	st.Reset(false)
	assert.Equal(t, "none", st.SecretStatus())
}

func TestBootSessions(t *testing.T) {
	var st tpm.State
	a := tpm.SentFile{Path: "a", Stage: "kernel", Digest: "aa"}
	b := tpm.SentFile{Path: "b", Stage: "image", Digest: "bb"}
	st.StartBoot(time.Now())
	st.RecordSent(a)
	st.RecordSent(a)
	assert.Len(t, st.Boot[0].Sent, 1, "duplicates are recorded once")
	st.StartBoot(time.Now())
	st.RecordSent(b)
	assert.ElementsMatch(t, []tpm.SentFile{a, b}, st.SentFiles(), "previous boot is kept")
	st.StartBoot(time.Now())
	assert.Equal(t, []tpm.SentFile{b}, st.SentFiles(), "only one previous boot is kept")
}

func TestStore(t *testing.T) {
	dir := t.TempDir()
	s := tpm.NewStore(filepath.Join(dir, "tpm"))

	st, err := s.Load("n1")
	require.NoError(t, err)
	assert.Nil(t, st.Enrolled)
	assert.False(t, s.Exists("n1"))

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.NoError(t, s.Update("n1", func(st *tpm.State) error {
				st.RecordSent(tpm.SentFile{Digest: hex.EncodeToString([]byte{byte(i)})})
				return nil
			}))
		}()
	}
	wg.Wait()
	st, err = s.Load("n1")
	require.NoError(t, err)
	assert.Len(t, st.Boot[0].Sent, 20, "concurrent updates are not lost")

	info, err := os.Stat(s.Path("n1"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	info, err = os.Stat(filepath.Join(dir, "tpm"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())

	nodes, err := s.Nodes()
	require.NoError(t, err)
	assert.Equal(t, []string{"n1"}, nodes)

	require.NoError(t, s.Delete("n1"))
	assert.False(t, s.Exists("n1"))

	for _, bad := range []string{"", "..", "a/b"} {
		assert.Error(t, s.Update(bad, func(*tpm.State) error { return nil }), bad)
	}
}

func TestStoreSavesOnError(t *testing.T) {
	s := tpm.NewStore(t.TempDir())
	rejected := &tpm.Error{Code: tpm.CodeEKChanged}
	err := s.Update("n1", func(st *tpm.State) error {
		st.Record(rejected, time.Now())
		return rejected
	})
	assert.ErrorIs(t, err, rejected)
	st, err := s.Load("n1")
	require.NoError(t, err)
	assert.Equal(t, tpm.CodeEKChanged, st.LastAttempt.Code, "state is saved when fn fails")
}

// An interrupted write leaves only a temporary file; the previous state is
// intact.
func TestStoreInterruptedWrite(t *testing.T) {
	dir := t.TempDir()
	s := tpm.NewStore(dir)
	require.NoError(t, s.Update("n1", func(st *tpm.State) error {
		st.RecordSent(tpm.SentFile{Digest: "aa"})
		return nil
	}))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".n1.json.123"), []byte("{trunc"), 0o600))
	st, err := s.Load("n1")
	require.NoError(t, err)
	assert.Len(t, st.Boot[0].Sent, 1)
}
