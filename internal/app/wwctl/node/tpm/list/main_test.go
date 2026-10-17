package list

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/warewulf/warewulf/internal/pkg/testenv"
	"github.com/warewulf/warewulf/internal/pkg/tpm"
	"github.com/warewulf/warewulf/internal/pkg/tpm/tpmtest"
	"github.com/warewulf/warewulf/internal/pkg/warewulfd"
)

func Test_List(t *testing.T) {
	warewulfd.SetNoDaemon()
	env := testenv.New(t)
	defer env.RemoveAll()
	tpm.ResetDefaultStore()
	defer tpm.ResetDefaultStore()
	env.WriteFile("etc/warewulf/nodes.conf", `nodes:
  n1:
    tpm: true
  n2:
    tpm: true
  n3: {}`)
	secret, err := tpmtest.MustNew().Attest(tpm.DefaultStore(), "n1")
	require.NoError(t, err)

	out := run(t)
	assert.Regexp(t, `NODE\s+STATE\s+MANUFACTURER\s+EK \(SHA256\)\s+LAST ATTEST\s+SECRET`, out)
	assert.Regexp(t, `n1\s+ENROLLED\s+Unknown\s+[0-9a-f]{16}\.\.\.\s+ok \S+ \S+\s+current`, out)
	assert.Regexp(t, `n2\s+UNENROLLED\s+--\s+--\s+--\s+none`, out)
	assert.NotContains(t, out, "n3", "nodes without TPM are not listed")

	var statuses []NodeTpmStatus
	require.NoError(t, json.Unmarshal([]byte(run(t, "--json")), &statuses))
	require.Len(t, statuses, 2)
	assert.Len(t, statuses[0].EKFingerprint, 64)
	assert.NotContains(t, run(t, "--json"), string(secret), "the secret is never shown")
}

func run(t *testing.T, args ...string) string {
	t.Helper()
	cmd := GetCommand()
	cmd.SetArgs(args)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	require.NoError(t, cmd.Execute())
	return buf.String()
}
