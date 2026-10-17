package report

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/warewulf/warewulf/internal/pkg/testenv"
	"github.com/warewulf/warewulf/internal/pkg/tpm"
	"github.com/warewulf/warewulf/internal/pkg/tpm/tpmtest"
	"github.com/warewulf/warewulf/internal/pkg/warewulfd"
)

func TestNodes(t *testing.T) {
	warewulfd.SetNoDaemon()
	env := testenv.New(t)
	defer env.RemoveAll()
	tpm.ResetDefaultStore()
	defer tpm.ResetDefaultStore()
	env.WriteFile("etc/warewulf/nodes.conf", `nodes:
  n1:
    tpm: true
  n2: {}
  n3: {}`)
	_, err := tpmtest.MustNew().Attest(tpm.DefaultStore(), "n3")
	require.NoError(t, err)

	ids, err := Nodes(nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"n1", "n3"}, ids, "nodes with TPM enabled or TPM state")

	ids, err = Nodes([]string{"n[1-2]"})
	require.NoError(t, err)
	assert.Equal(t, []string{"n1"}, ids)
}
