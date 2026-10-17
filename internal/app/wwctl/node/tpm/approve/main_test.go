package approve

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/warewulf/warewulf/internal/pkg/testenv"
	"github.com/warewulf/warewulf/internal/pkg/tpm"
	"github.com/warewulf/warewulf/internal/pkg/tpm/tpmtest"
	"github.com/warewulf/warewulf/internal/pkg/warewulfd"
)

func setup(t *testing.T) *tpm.Store {
	warewulfd.SetNoDaemon()
	env := testenv.New(t)
	t.Cleanup(env.RemoveAll)
	tpm.ResetDefaultStore()
	t.Cleanup(tpm.ResetDefaultStore)
	env.WriteFile("etc/warewulf/nodes.conf", `nodes:
  n1:
    tpm: true
  n2:
    tpm: true`)
	return tpm.DefaultStore()
}

func candidateID(t *testing.T, store *tpm.Store, node string) string {
	t.Helper()
	st, err := store.Load(node)
	require.NoError(t, err)
	require.NotNil(t, st.Candidate)
	return st.Candidate.ID()
}

func Test_Approve(t *testing.T) {
	store := setup(t)
	f := tpmtest.MustNew()
	_, err := f.Attest(store, "n1")
	require.NoError(t, err)

	out, err := run("n1", "--candidate", "0123456789abcdef")
	require.NoError(t, err)
	assert.Equal(t, "n1: nothing to approve\n", out)

	f.Extend(7, 0x800000e0, []byte("new Secure Boot db"))
	_, err = f.Attest(store, "n1")
	assert.Equal(t, tpm.CodeBaselineChanged, tpm.CodeOf(err))
	inspected := candidateID(t, store, "n1")

	_, err = run("n1")
	assert.ErrorContains(t, err, "--candidate "+inspected, "approving needs the candidate ID")

	// The candidate changes after the admin inspected it.
	f.Extend(7, 0x800000e0, []byte("another Secure Boot db"))
	_, err = f.Attest(store, "n1")
	assert.Equal(t, tpm.CodeBaselineChanged, tpm.CodeOf(err))
	_, err = run("n1", "--candidate", inspected)
	assert.ErrorContains(t, err, "check it again", "a replaced candidate is not approved")
	st, err := store.Load("n1")
	require.NoError(t, err)
	assert.Equal(t, "CANDIDATE", st.Status())

	current := candidateID(t, store, "n1")
	out, err = run("n1", "--candidate", current)
	require.NoError(t, err)
	assert.Regexp(t, `^n1: approved candidate [0-9a-f]{16}: baseline_changed, PCRs \[7\]; EK sha256 [0-9a-f]{64}\n$`, out)
	_, err = f.Attest(store, "n1")
	assert.NoError(t, err, "the approved baseline is accepted")

	_, err = run()
	assert.Error(t, err, "a node or --all is required")
	_, err = run("--all", "--candidate", current)
	assert.Error(t, err, "--candidate and --all don't combine")
	_, err = run("n[1-2]", "--candidate", current)
	assert.Error(t, err, "--candidate is for one node")
	_, err = run("n[1-2]")
	assert.ErrorContains(t, err, "give exactly one node with --candidate ID, or use --all")
}

func Test_ApproveWritesNothingWithoutCandidate(t *testing.T) {
	store := setup(t)
	out, err := run("n1", "--candidate", "0123456789abcdef")
	require.NoError(t, err)
	assert.Equal(t, "n1: nothing to approve\n", out)
	_, err = run("--all")
	require.NoError(t, err)
	assert.False(t, store.Exists("n1"), "no state file is created")
	assert.False(t, store.Exists("n2"))
}

func Test_ApproveAll(t *testing.T) {
	store := setup(t)
	firmware := tpmtest.MustNew()
	_, err := firmware.Attest(store, "n1")
	require.NoError(t, err)
	firmware.Extend(0, 0x80000008, []byte("firmware update"))
	_, err = firmware.Attest(store, "n1")
	require.Equal(t, tpm.CodeBaselineChanged, tpm.CodeOf(err))

	_, err = tpmtest.MustNew().Attest(store, "n2")
	require.NoError(t, err)
	_, err = tpmtest.MustNew().Attest(store, "n2")
	require.Equal(t, tpm.CodeEKChanged, tpm.CodeOf(err))

	out, err := run("--all")
	require.NoError(t, err)
	assert.Regexp(t, `(?m)^n1: approved candidate [0-9a-f]{16}: baseline_changed, PCRs \[0\]`, out)
	assert.Regexp(t, `(?m)^n2: skipped ek_changed candidate [0-9a-f]{16}; approve it individually with --candidate$`, out)
	st, err := store.Load("n2")
	require.NoError(t, err)
	assert.Equal(t, "CANDIDATE", st.Status(), "--all never approves a different TPM")
}

func run(args ...string) (string, error) {
	cmd := GetCommand()
	cmd.SetArgs(args)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(new(bytes.Buffer))
	err := cmd.Execute()
	return buf.String(), err
}
