package verify

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/warewulf/warewulf/internal/pkg/testenv"
	"github.com/warewulf/warewulf/internal/pkg/tpm"
	"github.com/warewulf/warewulf/internal/pkg/tpm/tpmtest"
	"github.com/warewulf/warewulf/internal/pkg/warewulfd"
)

func Test_Verify(t *testing.T) {
	warewulfd.SetNoDaemon()
	env := testenv.New(t)
	defer env.RemoveAll()
	tpm.ResetDefaultStore()
	defer tpm.ResetDefaultStore()
	env.WriteFile("etc/warewulf/nodes.conf", `nodes:
  n1:
    tpm: true`)
	f := tpmtest.MustNew()
	_, err := f.Attest(tpm.DefaultStore(), "n1")
	require.NoError(t, err)

	out, err := run("n1")
	require.NoError(t, err)
	assert.Contains(t, out, "n1: ENROLLED")
	assert.Contains(t, out, "evidence:      verified")
	assert.Contains(t, out, "sent kernel")

	out, err = run("--eventlog", "--pcr", "9", "n1")
	require.NoError(t, err)
	assert.Contains(t, out, "PCR[9]")
	assert.NotContains(t, out, "PCR[0]")

	// A firmware change is recorded as a candidate with a hint to approve it.
	f.Extend(0, 0x80000008, []byte("firmware update"))
	_, err = f.Attest(tpm.DefaultStore(), "n1")
	require.Error(t, err)
	out, err = run("n1")
	require.NoError(t, err)
	assert.Contains(t, out, "n1: CANDIDATE")
	assert.Regexp(t, `candidate: +[0-9a-f]{16}: baseline_changed, PCRs \[0\] differ`, out)
	assert.Regexp(t, `wwctl node tpm approve n1 --candidate [0-9a-f]{16}`, out)

	// Tampered stored evidence fails verification.
	require.NoError(t, tpm.DefaultStore().Update("n1", func(st *tpm.State) error {
		st.Current.Evidence.PCRs[0] = make([]byte, 32)
		return nil
	}))
	out, err = run("n1")
	assert.Error(t, err)
	assert.Contains(t, out, "evidence:      FAILED")

	_, err = run("--pcr", "9", "n1")
	assert.Error(t, err, "--pcr requires --eventlog")
}

// Evidence behind a secret the node hasn't used is unproven, and evidence
// sent only for a change is never shown.
func Test_VerifyUnconfirmed(t *testing.T) {
	warewulfd.SetNoDaemon()
	env := testenv.New(t)
	defer env.RemoveAll()
	tpm.ResetDefaultStore()
	defer tpm.ResetDefaultStore()
	env.WriteFile("etc/warewulf/nodes.conf", `nodes:
  n1:
    tpm: true`)
	store := tpm.DefaultStore()
	f := tpmtest.MustNew()
	_, err := f.Attest(store, "n1")
	require.NoError(t, err)

	// A change attempt that was never confirmed, as anyone could send.
	require.NoError(t, store.Update("n1", func(st *tpm.State) error {
		st.Reset(false)
		st.AddConfirmation([]byte("forged"), &tpm.Candidate{Reason: tpm.CodeBaselineChanged}, time.Now())
		return nil
	}))
	out, err := run("n1")
	require.NoError(t, err)
	assert.NotContains(t, out, "evidence:", "an unconfirmed change attempt is not shown as evidence")

	// A pending runtime secret the node hasn't used yet.
	ev, err := f.Evidence(make([]byte, 32), nil)
	require.NoError(t, err)
	require.NoError(t, store.Update("n1", func(st *tpm.State) error {
		v := &tpm.Verified{AKFingerprint: tpm.Fingerprint(ev.AK.Public)}
		st.AddPending([]byte("pending"), ev, v, st.SentFiles(), nil, time.Now())
		return nil
	}))
	out, _ = run("n1")
	assert.Contains(t, out, "unconfirmed: the node hasn't used this secret yet")
}

func Test_VerifyFile(t *testing.T) {
	store := tpm.NewStore(t.TempDir())
	_, err := tpmtest.MustNew().Attest(store, "n1")
	require.NoError(t, err)

	out, err := run("--file", store.Path("n1"))
	require.NoError(t, err)
	assert.Contains(t, out, store.Path("n1")+": ENROLLED")
	assert.Contains(t, out, "evidence:      verified")

	other := filepath.Join(t.TempDir(), "other.json")
	require.NoError(t, os.WriteFile(other, []byte(`{"current": {}}`), 0o600))
	_, err = run("--file", other)
	assert.ErrorContains(t, err, "not a TPM state file")

	_, err = run("--file", filepath.Join(t.TempDir(), "missing.json"))
	assert.Error(t, err)

	_, err = run("--file", store.Path("n1"), "n1")
	assert.ErrorContains(t, err, "not both")
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
