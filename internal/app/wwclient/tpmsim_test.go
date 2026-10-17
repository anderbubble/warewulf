//go:build tpmsim

// Tests of TPM attestation against a software TPM (swtpm), run with
// `go test -tags tpmsim`. They exercise what the pure-Go fake TPM cannot:
// real TPM commands, AK creation and reloading, and credential activation.

package wwclient

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/google/go-attestation/attest"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	warewulfconf "github.com/warewulf/warewulf/internal/pkg/config"
	"github.com/warewulf/warewulf/internal/pkg/testenv"
	"github.com/warewulf/warewulf/internal/pkg/tpm"
	"github.com/warewulf/warewulf/internal/pkg/tpm/tpmtest"
	"github.com/warewulf/warewulf/internal/pkg/warewulfd"
)

func TestSwtpmAttestation(t *testing.T) {
	sw := tpmtest.StartSwtpm(t)

	env := testenv.New(t)
	defer env.RemoveAll()
	env.WriteFile("/etc/warewulf/nodes.conf", `nodes:
  n1:
    tpm: true
    network devices:
      default:
        hwaddr: `+testWWID)
	require.NoError(t, warewulfd.LoadNodeDB())
	tpm.ResetDefaultStore()
	defer tpm.ResetDefaultStore()
	conf := warewulfconf.Get()
	secureFalse := false
	conf.Warewulf.SecureP = &secureFalse
	overlayDir := filepath.Join(conf.Paths.OverlayProvisiondir(), "n1")
	require.NoError(t, os.MkdirAll(overlayDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(overlayDir, "__RUNTIME__.img"), []byte("runtime overlay"), 0o600))

	// Firmware and a GRUB boot of a kernel the server sent.
	for _, pcr := range tpm.BaselinePCRs {
		require.NoError(t, sw.Extend(pcr, 0x80000008, []byte{byte(pcr)}))
	}
	kernel := []byte("vmlinuz contents")
	require.NoError(t, sw.Extend(tpm.PCRBootFiles, tpmtest.EvIPL, kernel))
	sum := sha256.Sum256(kernel)
	require.NoError(t, tpm.DefaultStore().Update("n1", func(st *tpm.State) error {
		st.StartBoot(time.Now())
		st.RecordSent(tpm.SentFile{Path: "vmlinuz", Stage: tpm.StageKernel, Digest: hex.EncodeToString(sum[:])})
		return nil
	}))

	mux := http.NewServeMux()
	mux.HandleFunc("/tpm/", warewulfd.HandleTPM)
	mux.HandleFunc("/runtime/", warewulfd.HandleRuntimeOverlay)
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()
	saved := webclient
	webclient = srv.Client()
	defer func() { webclient = saved }()
	host, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	require.NoError(t, err)
	p, err := strconv.Atoi(port)
	require.NoError(t, err)
	o := updateOptions{scheme: "https", ipaddr: host, port: p, wwid: testWWID, localUUID: uuid.New()}

	akPath := filepath.Join(t.TempDir(), "tpm", "ak.blob")
	open := openTPMAttestor(akPath, sw.Open)

	runtime := func(secret []byte) int {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, o.url("runtime/"+testWWID), nil)
		require.NoError(t, err)
		if secret != nil {
			req.Header.Set(tpm.SecretHeader, hex.EncodeToString(secret))
		}
		resp, err := webclient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode == http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			assert.Equal(t, "runtime overlay", string(body))
		}
		return resp.StatusCode
	}

	// Boot: the server asks for attestation, and the TPM's secret unlocks
	// the runtime overlay.
	assert.ErrorIs(t, updateSystem(o), errTPMRequired)
	first, err := attestTPM(webclient, o, open)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, runtime(first))
	st, err := tpm.DefaultStore().Load("n1")
	require.NoError(t, err)
	assert.Equal(t, "ENROLLED", st.Status())
	assert.FileExists(t, akPath, "the AK is saved")

	// Attesting again (with the saved AK) leaves the running secret valid
	// until the new one is used.
	second, err := attestTPM(webclient, o, open)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, runtime(first))
	assert.Equal(t, http.StatusOK, runtime(second))
	assert.Equal(t, http.StatusUnauthorized, runtime(first))

	// A re-provisioned node has a new AK; the same TPM is still accepted.
	require.NoError(t, os.Remove(akPath))
	third, err := attestTPM(webclient, o, open)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, runtime(third))

	// A genuine TPM quote over an empty SHA-1 selection covers none of the
	// SHA-256 PCRs reported with it, so they must not be trusted.
	a, err := open()
	require.NoError(t, err)
	ta := a.(*tpmAttestor)
	var nr tpm.NonceResponse
	require.NoError(t, postJSON(webclient, o.url("tpm/nonce/"+testWWID), nil, nil, &nr))
	ev, err := ta.Evidence(nr.Nonce)
	require.NoError(t, err)
	q, err := ta.ak.QuotePCRs(ta.t, nr.Nonce, attest.HashSHA1, []int{})
	require.NoError(t, err)
	ev.Quote, ev.Signature = q.Quote, q.Signature
	var qr tpm.QuoteResponse
	err = postJSON(webclient, o.url("tpm/quote/"+testWWID), nil, ev, &qr)
	assert.ErrorContains(t, err, string(tpm.CodeQuoteInvalid))
	require.NoError(t, a.Close())

	// A firmware change is refused. The node proves it holds the TPM by
	// decrypting the credential issued for the change, which records it as a
	// candidate until approved.
	require.NoError(t, sw.Extend(0, 0x80000008, []byte("firmware update")))
	_, err = attestTPM(webclient, o, open)
	require.Error(t, err)
	assert.Contains(t, err.Error(), string(tpm.CodeBaselineChanged))
	assert.Contains(t, err.Error(), "awaiting approval")
	st, err = tpm.DefaultStore().Load("n1")
	require.NoError(t, err)
	require.NotNil(t, st.Candidate, "the confirmed change is recorded")
	assert.Equal(t, []int{0}, st.Candidate.Changed)
	require.NoError(t, tpm.DefaultStore().Update("n1", func(st *tpm.State) error { return st.Approve(time.Now()) }))
	fourth, err := attestTPM(webclient, o, open)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, runtime(fourth))

	// Someone claiming this node's EK, which is public, with an AK in another
	// TPM can't decrypt the credential, so no change is recorded.
	other := tpmtest.StartSwtpm(t)
	require.NoError(t, other.Extend(0, 0x80000008, []byte("attacker's firmware")))
	require.NoError(t, other.Extend(tpm.PCRBootFiles, tpmtest.EvIPL, kernel))
	attacker, err := openTPMAttestor(filepath.Join(t.TempDir(), "ak.blob"), other.Open)()
	require.NoError(t, err)
	require.NoError(t, postJSON(webclient, o.url("tpm/nonce/"+testWWID), nil, nil, &nr))
	ev, err = attacker.Evidence(nr.Nonce)
	require.NoError(t, err)
	st, err = tpm.DefaultStore().Load("n1")
	require.NoError(t, err)
	ev.EKPub, ev.EKCert = st.Enrolled.EKPub, nil
	err = postJSON(webclient, o.url("tpm/quote/"+testWWID), nil, ev, &qr)
	var se *statusError
	require.ErrorAs(t, err, &se)
	assert.Equal(t, http.StatusConflict, se.code)
	require.NoError(t, json.Unmarshal(se.body, &qr))
	_, err = attacker.Activate(qr.Credential)
	assert.Error(t, err, "another TPM can't decrypt a credential bound to this node's EK")
	require.NoError(t, attacker.Close())
	st, err = tpm.DefaultStore().Load("n1")
	require.NoError(t, err)
	assert.Nil(t, st.Candidate, "no change is recorded")
	assert.Equal(t, http.StatusOK, runtime(fourth), "the node is unaffected")

	// After a reset the node is asked to attest again, and can.
	require.NoError(t, tpm.DefaultStore().Update("n1", func(st *tpm.State) error { st.Reset(false); return nil }))
	assert.Equal(t, http.StatusUnauthorized, runtime(fourth))
	fifth, err := attestTPM(webclient, o, open)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, runtime(fifth))

	// A boot through something other than the files this server sent fails.
	require.NoError(t, sw.Extend(tpm.PCRBootFiles, tpmtest.EvIPL, []byte("someone else's initrd")))
	_, err = attestTPM(webclient, o, open)
	require.Error(t, err)
	assert.Contains(t, err.Error(), string(tpm.CodeBootFilesUnknown))
}
