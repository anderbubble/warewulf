package warewulfd

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-attestation/attest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	warewulfconf "github.com/warewulf/warewulf/internal/pkg/config"
	"github.com/warewulf/warewulf/internal/pkg/testenv"
	"github.com/warewulf/warewulf/internal/pkg/tpm"
	"github.com/warewulf/warewulf/internal/pkg/tpm/tpmtest"
)

const (
	tpmNode   = "00:00:00:00:00:01"
	plainNode = "00:00:00:00:00:02"
)

var kernelData = []byte("kernel")

// setupTPM configures n1 (TPM) and n2 (no TPM) with built overlays, and
// captures issued secrets.
func setupTPM(t *testing.T) (secrets *[][]byte) {
	env := testenv.New(t)
	t.Cleanup(env.RemoveAll)
	env.WriteFile("/etc/warewulf/nodes.conf", `nodes:
  n1:
    tpm: true
    network devices:
      default:
        hwaddr: `+tpmNode+`
  n2:
    network devices:
      default:
        hwaddr: `+plainNode)
	require.NoError(t, LoadNodeDB())
	tpm.ResetDefaultStore()
	t.Cleanup(tpm.ResetDefaultStore)
	nonces = newNonceStore()

	conf := warewulfconf.Get()
	secureFalse := false
	conf.Warewulf.SecureP = &secureFalse
	for _, n := range []string{"n1", "n2"} {
		dir := path.Join(conf.Paths.OverlayProvisiondir(), n)
		require.NoError(t, os.MkdirAll(dir, 0o700))
		require.NoError(t, os.WriteFile(path.Join(dir, "__SYSTEM__.img"), []byte("system overlay"), 0o600))
		require.NoError(t, os.WriteFile(path.Join(dir, "__RUNTIME__.img"), []byte("runtime overlay"), 0o600))
	}

	secrets = &[][]byte{}
	saved := generateActivation
	generateActivation = func(p *attest.ActivationParameters) ([]byte, *attest.EncryptedCredential, error) {
		secret, ec, err := p.Generate()
		if err == nil {
			*secrets = append(*secrets, secret)
		}
		return secret, ec, err
	}
	t.Cleanup(func() { generateActivation = saved })
	return secrets
}

type tpmRequest struct {
	method, url string
	body        []byte
	header      map[string]string
	plainHTTP   bool
}

func do(t *testing.T, h http.HandlerFunc, r tpmRequest) *http.Response {
	t.Helper()
	if r.method == "" {
		r.method = http.MethodPost
	}
	req := httptest.NewRequest(r.method, r.url, bytes.NewReader(r.body))
	req.RemoteAddr = "10.10.10.10:987"
	if !r.plainHTTP {
		req.TLS = &tls.ConnectionState{}
	}
	for k, v := range r.header {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h(w, req)
	return w.Result()
}

// bootTPM measures a GRUB boot of the kernel into f and records the kernel as
// sent to n1, as serving it would.
func bootTPM(t *testing.T) *tpmtest.TPM {
	f := tpmtest.MustNew()
	f.Extend(0, 0x80000008, []byte("firmware"))
	f.Extend(tpm.PCRBootFiles, tpmtest.EvIPL, kernelData)
	sum := sha256.Sum256(kernelData)
	require.NoError(t, tpm.DefaultStore().Update("n1", func(st *tpm.State) error {
		st.StartBoot(time.Now())
		st.RecordSent(tpm.SentFile{Path: "vmlinuz", Stage: tpm.StageKernel, Digest: hex.EncodeToString(sum[:])})
		return nil
	}))
	return f
}

func getNonce(t *testing.T, node string) []byte {
	t.Helper()
	res := do(t, HandleTPM, tpmRequest{url: "/tpm/nonce/" + node})
	require.Equal(t, http.StatusOK, res.StatusCode)
	var body TPMNonceResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Len(t, body.Nonce, nonceSize)
	return body.Nonce
}

func postQuote(t *testing.T, f *tpmtest.TPM, nonce []byte) *http.Response {
	t.Helper()
	ev, err := f.Evidence(nonce, nil)
	require.NoError(t, err)
	return postEvidence(t, ev)
}

func postEvidence(t *testing.T, ev *tpm.Evidence) *http.Response {
	t.Helper()
	body, err := json.Marshal(ev)
	require.NoError(t, err)
	return do(t, HandleTPM, tpmRequest{url: "/tpm/quote/" + tpmNode, body: body})
}

func confirm(t *testing.T, secret []byte) *http.Response {
	t.Helper()
	return do(t, HandleTPM, tpmRequest{url: "/tpm/confirm/" + tpmNode, header: map[string]string{TPMSecretHeader: hex.EncodeToString(secret)}})
}

func loadState(t *testing.T) *tpm.State {
	t.Helper()
	st, err := tpm.DefaultStore().Load("n1")
	require.NoError(t, err)
	return st
}

func runtime(t *testing.T, secret []byte) *http.Response {
	t.Helper()
	header := map[string]string{}
	if secret != nil {
		header[TPMSecretHeader] = hex.EncodeToString(secret)
	}
	return do(t, HandleRuntimeOverlay, tpmRequest{method: http.MethodGet, url: "/runtime/" + tpmNode, header: header})
}

func errorCode(t *testing.T, res *http.Response) tpm.Code {
	t.Helper()
	var body TPMErrorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	return body.Error
}

func TestTPMAttestation(t *testing.T) {
	secrets := setupTPM(t)

	res := runtime(t, nil)
	assert.Equal(t, http.StatusUnauthorized, res.StatusCode, "a TPM node without a secret is refused")
	assert.Equal(t, "required", res.Header.Get(TPMRequiredHeader), "and told to attest")

	f := bootTPM(t)
	res = postQuote(t, f, getNonce(t, tpmNode))
	require.Equal(t, http.StatusOK, res.StatusCode)
	var body TPMQuoteResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.NotNil(t, body.Credential)
	require.Len(t, *secrets, 1)
	first := (*secrets)[0]
	assert.Nil(t, loadState(t).Enrolled, "nothing is enrolled until the node uses the secret")

	res = runtime(t, first)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	data, _ := io.ReadAll(res.Body)
	assert.Equal(t, "runtime overlay", string(data))
	assert.Equal(t, "TPM", statusDB.Nodes["n1"].Security)
	assert.NotNil(t, loadState(t).Enrolled, "using the secret enrolls the node")

	res = runtime(t, []byte("wrong"))
	assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
	assert.Equal(t, "required", res.Header.Get(TPMRequiredHeader))

	// Another attestation issues a new secret without disturbing the one in
	// use; the node's secret is replaced only when the new one is used.
	res = postQuote(t, f, getNonce(t, tpmNode))
	require.Equal(t, http.StatusOK, res.StatusCode)
	second := (*secrets)[1]
	assert.Equal(t, http.StatusOK, runtime(t, first).StatusCode)
	assert.Equal(t, http.StatusOK, runtime(t, second).StatusCode)
	assert.Equal(t, http.StatusUnauthorized, runtime(t, first).StatusCode)

	st, err := tpm.DefaultStore().Load("n1")
	require.NoError(t, err)
	assert.Equal(t, "ENROLLED", st.Status())
	assert.True(t, st.LastAttempt.OK())
	raw, err := os.ReadFile(tpm.DefaultStore().Path("n1"))
	require.NoError(t, err)
	assert.NotContains(t, string(raw), hex.EncodeToString(second), "secrets are not stored")
}

func TestTPMQuoteRejects(t *testing.T) {
	setupTPM(t)
	f := bootTPM(t)

	t.Run("replayed nonce", func(t *testing.T) {
		n := getNonce(t, tpmNode)
		assert.Equal(t, http.StatusOK, postQuote(t, f, n).StatusCode)
		assert.Equal(t, http.StatusUnauthorized, postQuote(t, f, n).StatusCode)
	})
	t.Run("nonce never issued", func(t *testing.T) {
		assert.Equal(t, http.StatusUnauthorized, postQuote(t, f, make([]byte, nonceSize)).StatusCode)
	})
	t.Run("malformed body", func(t *testing.T) {
		res := do(t, HandleTPM, tpmRequest{url: "/tpm/quote/" + tpmNode, body: []byte("{")})
		assert.Equal(t, http.StatusBadRequest, res.StatusCode)
		assert.Equal(t, tpm.CodeBadEvidence, errorCode(t, res))
	})
	t.Run("oversized body", func(t *testing.T) {
		body := []byte(`{"nonce":"` + strings.Repeat("A", maxEvidenceBytes) + `"}`)
		res := do(t, HandleTPM, tpmRequest{url: "/tpm/quote/" + tpmNode, body: body})
		assert.Equal(t, http.StatusBadRequest, res.StatusCode)
	})
	t.Run("boot file not sent by this server", func(t *testing.T) {
		f.Extend(tpm.PCRBootFiles, tpmtest.EvIPL, []byte("rogue initrd"))
		res := postQuote(t, f, getNonce(t, tpmNode))
		assert.Equal(t, http.StatusForbidden, res.StatusCode)
		assert.Equal(t, tpm.CodeBootFilesUnknown, errorCode(t, res))
		assert.Equal(t, "TPM-FAIL", statusDB.Nodes["n1"].Security)
	})
}

// enrolled sets up n1 enrolled with a booted TPM.
func enrolled(t *testing.T) (*tpmtest.TPM, *[][]byte) {
	t.Helper()
	secrets := setupTPM(t)
	f := bootTPM(t)
	require.Equal(t, http.StatusOK, postQuote(t, f, getNonce(t, tpmNode)).StatusCode)
	require.Equal(t, http.StatusOK, runtime(t, (*secrets)[0]).StatusCode)
	return f, secrets
}

func TestTPMChange(t *testing.T) {
	f, secrets := enrolled(t)
	current := (*secrets)[0]

	f.Extend(0, 0x80000008, []byte("firmware update"))
	res := postQuote(t, f, getNonce(t, tpmNode))
	assert.Equal(t, http.StatusConflict, res.StatusCode)
	var body TPMQuoteResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, tpm.CodeBaselineChanged, body.Error)
	assert.NotNil(t, body.Credential, "a change gets a credential, to prove the node holds the TPM")
	assert.Nil(t, loadState(t).Candidate, "nothing is recorded before that proof")
	assert.True(t, loadState(t).LastAttempt.OK(), "an unconfirmed change isn't shown as the last attempt")
	assert.Equal(t, "TPM", statusDB.Nodes["n1"].Security, "or in the node's status")

	changed := (*secrets)[1]
	assert.Equal(t, http.StatusUnauthorized, runtime(t, changed).StatusCode, "a change's secret doesn't unlock the runtime overlay")
	assert.Equal(t, http.StatusOK, runtime(t, current).StatusCode, "and the running secret still works")

	assert.Equal(t, http.StatusUnauthorized, confirm(t, []byte("wrong")).StatusCode)
	res = confirm(t, changed)
	require.Equal(t, http.StatusOK, res.StatusCode)
	var confirmed TPMConfirmResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&confirmed))
	st := loadState(t)
	require.NotNil(t, st.Candidate)
	assert.Equal(t, st.Candidate.ID(), confirmed.Candidate)
	assert.Equal(t, []int{0}, st.Candidate.Changed)
	assert.Equal(t, tpm.CodeBaselineChanged, st.LastAttempt.Code, "the confirmed change is the last attempt")
	assert.Equal(t, "TPM-FAIL", statusDB.Nodes["n1"].Security)
	assert.Equal(t, http.StatusUnauthorized, confirm(t, changed).StatusCode, "a confirmation is single-use")
}

// Anyone with the node's address and asset key can claim the enrolled EK,
// which is public, and sign a quote with an AK in some other TPM. Without
// that TPM they can't decrypt the credential, so no change is recorded.
func TestTPMChangeClaimingAnotherEK(t *testing.T) {
	f, _ := enrolled(t)
	before := loadState(t)

	attacker := tpmtest.MustNew()
	attacker.Extend(0, 0x80000008, []byte("attacker's firmware"))
	attacker.Extend(tpm.PCRBootFiles, tpmtest.EvIPL, kernelData)
	ev, err := attacker.Evidence(getNonce(t, tpmNode), nil)
	require.NoError(t, err)
	ev.EKPub = f.EKPub()

	res := postEvidence(t, ev)
	assert.Equal(t, http.StatusConflict, res.StatusCode)
	assert.Equal(t, tpm.CodeBaselineChanged, errorCode(t, res))
	assert.Equal(t, http.StatusUnauthorized, confirm(t, []byte("guess")).StatusCode)

	after := loadState(t)
	assert.Nil(t, after.Candidate, "no candidate is recorded")
	assert.Equal(t, before.Enrolled, after.Enrolled, "the enrollment is unchanged")
	assert.Equal(t, before.LastAttempt, after.LastAttempt, "the last attempt is unchanged")
	assert.Equal(t, "TPM", statusDB.Nodes["n1"].Security, "the node's status is unchanged")
}

func TestTPMRouteAccess(t *testing.T) {
	setupTPM(t)

	assert.Equal(t, http.StatusNotFound, do(t, HandleTPM, tpmRequest{url: "/tpm/nonce/" + plainNode}).StatusCode,
		"a node without TPM has no TPM routes")
	assert.Equal(t, http.StatusForbidden, do(t, HandleTPM, tpmRequest{url: "/tpm/nonce/" + tpmNode, plainHTTP: true}).StatusCode,
		"TPM routes require TLS")
	assert.Equal(t, http.StatusMethodNotAllowed, do(t, HandleTPM, tpmRequest{method: http.MethodGet, url: "/tpm/nonce/" + tpmNode}).StatusCode)
	assert.Equal(t, http.StatusOK, do(t, HandleTPM, tpmRequest{url: "/tpm/nonce?wwid=" + tpmNode}).StatusCode,
		"wwid may be given as a query parameter")

	conf := warewulfconf.Get()
	secureTrue := true
	conf.Warewulf.SecureP = &secureTrue
	req := httptest.NewRequest(http.MethodPost, "/tpm/nonce/"+tpmNode, nil)
	req.RemoteAddr = "10.10.10.10:40000"
	req.TLS = &tls.ConnectionState{}
	w := httptest.NewRecorder()
	HandleTPM(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code, "with warewulf:secure, TPM routes require a privileged port")

	res := do(t, HandleRuntimeOverlay, tpmRequest{method: http.MethodGet, url: "/runtime/" + tpmNode, plainHTTP: true})
	assert.Equal(t, http.StatusForbidden, res.StatusCode, "a TPM node's runtime overlay is refused without TLS")
	assert.Empty(t, res.Header.Get(TPMRequiredHeader))

	res = do(t, HandleRuntimeOverlay, tpmRequest{method: http.MethodGet, url: "/runtime/" + plainNode, plainHTTP: true})
	assert.Equal(t, http.StatusOK, res.StatusCode, "nodes without TPM are unaffected")
}

func TestTPMRecordsBootFiles(t *testing.T) {
	setupTPM(t)

	for _, node := range []string{tpmNode, plainNode} {
		res := do(t, HandleSystemOverlay, tpmRequest{method: http.MethodGet, url: "/system/" + node, plainHTTP: true})
		require.Equal(t, http.StatusOK, res.StatusCode)
	}
	st, err := tpm.DefaultStore().Load("n1")
	require.NoError(t, err)
	sum := sha256.Sum256([]byte("system overlay"))
	require.Len(t, st.SentFiles(), 1)
	assert.Equal(t, "system", st.SentFiles()[0].Stage)
	assert.Equal(t, hex.EncodeToString(sum[:]), st.SentFiles()[0].Digest)

	assert.False(t, tpm.DefaultStore().Exists("n2"), "nothing is recorded for nodes without TPM")

	res := do(t, HandleRuntimeOverlay, tpmRequest{method: http.MethodGet, url: "/runtime/" + plainNode, plainHTTP: true})
	require.Equal(t, http.StatusOK, res.StatusCode)
	assert.False(t, tpm.DefaultStore().Exists("n2"))
}

func TestNonceStore(t *testing.T) {
	s := newNonceStore()
	now := time.Now()

	n, err := s.issue("n1", now)
	require.NoError(t, err)
	assert.False(t, s.consume("n2", n, now), "nonces are per node")
	assert.True(t, s.consume("n1", n, now))
	assert.False(t, s.consume("n1", n, now), "nonces are single use")

	n, err = s.issue("n1", now)
	require.NoError(t, err)
	assert.False(t, s.consume("n1", n, now.Add(nonceTTL)), "nonces expire")

	var first []byte
	for i := range maxNoncesPerNode + 1 {
		n, err := s.issue("n1", now)
		require.NoError(t, err)
		if i == 0 {
			first = n
		}
	}
	assert.False(t, s.consume("n1", first, now), "the oldest nonce is dropped beyond the per-node limit")
}

func TestDigestCache(t *testing.T) {
	var calls atomic.Int32
	c := newDigestCache(func(f *os.File) (string, error) {
		calls.Add(1)
		time.Sleep(10 * time.Millisecond)
		data, err := io.ReadAll(f)
		sum := sha256.Sum256(data)
		return hex.EncodeToString(sum[:]), err
	})
	name := path.Join(t.TempDir(), "image")
	require.NoError(t, os.WriteFile(name, []byte("one"), 0o600))

	done := make(chan string)
	for range 8 {
		go func() {
			d, err := c.sumPath(name)
			assert.NoError(t, err)
			done <- d
		}()
	}
	for range 8 {
		<-done
	}
	assert.Equal(t, int32(1), calls.Load(), "concurrent requests share one hash")

	_, err := c.sumPath(name)
	require.NoError(t, err)
	assert.Equal(t, int32(1), calls.Load(), "cached")

	require.NoError(t, os.WriteFile(name, []byte("two!"), 0o600))
	d, err := c.sumPath(name)
	require.NoError(t, err)
	sum := sha256.Sum256([]byte("two!"))
	assert.Equal(t, hex.EncodeToString(sum[:]), d, "a changed file is hashed again")
	assert.Equal(t, int32(2), calls.Load())
}
