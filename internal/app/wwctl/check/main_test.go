package check

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	warewulfconf "github.com/warewulf/warewulf/internal/pkg/config"
	"github.com/warewulf/warewulf/internal/pkg/testenv"
	"github.com/warewulf/warewulf/internal/pkg/tpm"
	"github.com/warewulf/warewulf/internal/pkg/warewulfd"
)

func Test_Check(t *testing.T) {
	warewulfd.SetNoDaemon()
	env := testenv.New(t)
	defer env.RemoveAll()
	tpm.ResetDefaultStore()
	defer tpm.ResetDefaultStore()
	conf := warewulfconf.Get()
	yes, no := true, false
	conf.Warewulf.TLSEnabledP, conf.Warewulf.GrubBootP = &yes, &yes

	env.WriteFile("etc/warewulf/nodes.conf", `nodes:
  n1:
    tpm: true
  n2:
    tpm: true`)
	out, err := run()
	assert.NoError(t, err)
	assert.Equal(t, "No problems found.\n", out)

	conf.Warewulf.GrubBootP = &no
	out, err = run()
	assert.NoError(t, err, "warnings alone succeed")
	assert.Contains(t, out, "WARN   TPM attestation requires GRUB boot")
	assert.Contains(t, out, "       nodes: n[1-2]\n")
	assert.Contains(t, out, "0 error(s), 1 warning(s)\n")

	conf.Warewulf.TLSEnabledP = &no
	out, err = run()
	assert.Error(t, err, "an error fails the command")
	assert.Contains(t, out, "ERROR  TPM attestation requires TLS")
	assert.Contains(t, out, "1 error(s), 1 warning(s)\n")
}

func run() (string, error) {
	cmd := GetCommand()
	cmd.SetArgs(nil)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(new(bytes.Buffer))
	err := cmd.Execute()
	return buf.String(), err
}
