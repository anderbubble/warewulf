package reset

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

func Test_Reset(t *testing.T) {
	warewulfd.SetNoDaemon()
	env := testenv.New(t)
	defer env.RemoveAll()
	tpm.ResetDefaultStore()
	defer tpm.ResetDefaultStore()
	env.WriteFile("etc/warewulf/nodes.conf", `nodes:
  n1:
    tpm: true`)
	store := tpm.DefaultStore()
	_, err := tpmtest.MustNew().Attest(store, "n1")
	require.NoError(t, err)

	out, err := run("n1")
	require.NoError(t, err)
	assert.Equal(t, "n1: TPM secret reset; it will attest again\n", out)
	st, err := store.Load("n1")
	require.NoError(t, err)
	assert.Equal(t, "none", st.SecretStatus())
	assert.Equal(t, "ENROLLED", st.Status(), "enrollment is kept")

	out, err = run("--enrollment", "n1")
	require.NoError(t, err)
	assert.Equal(t, "n1: TPM enrollment reset; its next attestation enrolls it\n", out)
	st, err = store.Load("n1")
	require.NoError(t, err)
	assert.Equal(t, "UNENROLLED", st.Status())

	_, err = run("n9")
	assert.Error(t, err)

	require.NoError(t, store.Delete("n1"))
	out, err = run("n1")
	require.NoError(t, err)
	assert.Equal(t, "n1: no TPM state to reset\n", out)
	assert.False(t, store.Exists("n1"), "no state file is created")
	_, err = run()
	assert.Error(t, err, "a node is required")
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
