package check

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	warewulfconf "github.com/warewulf/warewulf/internal/pkg/config"
	"github.com/warewulf/warewulf/internal/pkg/node"
	"github.com/warewulf/warewulf/internal/pkg/testenv"
	"github.com/warewulf/warewulf/internal/pkg/tpm"
	"github.com/warewulf/warewulf/internal/pkg/tpm/tpmtest"
)

func nodes(t *testing.T, env *testenv.TestEnv, conf string) []node.Node {
	t.Helper()
	env.WriteFile("etc/warewulf/nodes.conf", conf)
	db, err := node.New()
	require.NoError(t, err)
	n, err := db.FindAllNodes()
	require.NoError(t, err)
	return n
}

func TestTPMCheck(t *testing.T) {
	env := testenv.New(t)
	defer env.RemoveAll()
	tpm.ResetDefaultStore()
	defer tpm.ResetDefaultStore()
	conf := warewulfconf.Get()
	yes, no := true, false

	tpmNodes := nodes(t, env, `nodeprofiles:
  secure:
    tpm: true
nodes:
  n1:
    profiles: [secure]
  n2:
    tpm: true
  n3: {}`)

	conf.Warewulf.TLSEnabledP, conf.Warewulf.GrubBootP = &no, &no
	findings := Run(conf, tpmNodes)
	require.Len(t, findings, 2)
	assert.Equal(t, Error, findings[0].Severity)
	assert.Contains(t, findings[0].Message, "requires TLS")
	assert.Equal(t, []string{"n1", "n2"}, findings[0].Nodes, "TPM set on a profile counts; nodes without TPM don't")
	assert.Equal(t, Warning, findings[1].Severity)
	assert.Contains(t, findings[1].Message, "requires GRUB boot")

	conf.Warewulf.TLSEnabledP, conf.Warewulf.GrubBootP = &yes, &yes
	assert.Empty(t, Run(conf, tpmNodes))

	f := tpmtest.MustNew()
	_, err := f.Attest(tpm.DefaultStore(), "n2")
	require.NoError(t, err)
	f.Extend(0, 0x80000008, []byte("firmware update"))
	_, err = f.Attest(tpm.DefaultStore(), "n2")
	require.Error(t, err)
	findings = Run(conf, tpmNodes)
	require.Len(t, findings, 1)
	assert.Equal(t, Warning, findings[0].Severity)
	assert.Contains(t, findings[0].Message, "TPM change awaiting approval (baseline_changed, candidate ")
	assert.Equal(t, []string{"n2"}, findings[0].Nodes)

	assert.Empty(t, Run(conf, nodes(t, env, `nodes:
  n3: {}`)), "no TPM nodes, no TPM findings")
}
