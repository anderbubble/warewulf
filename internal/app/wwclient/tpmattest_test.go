package wwclient

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/google/go-attestation/attest"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/warewulf/warewulf/internal/pkg/tpm"
	"github.com/warewulf/warewulf/internal/pkg/tpm/tpmtest"
)

const testWWID = "00:00:00:00:00:01"

var testSecret = []byte("runtime secret")

// fakeAttestor quotes with a software TPM and "decrypts" a credential by
// returning its Secret field, which the fake server fills with the secret.
type fakeAttestor struct{ f *tpmtest.TPM }

func (a fakeAttestor) Evidence(nonce []byte) (*tpm.Evidence, error) { return a.f.Evidence(nonce, nil) }
func (a fakeAttestor) Activate(c *attest.EncryptedCredential) ([]byte, error) {
	return c.Secret, nil
}
func (a fakeAttestor) Close() error { return nil }

func openFake() (attestor, error) { return fakeAttestor{tpmtest.MustNew()}, nil }

// fakeServer stands in for warewulfd's TPM and runtime routes.
type fakeServer struct {
	nonce     []byte
	quoteCode tpm.Code
	gotQuery  map[string]string
	runtimeOK bool
	secrets   [][]byte
	confirmed [][]byte
}

func (s *fakeServer) start(t *testing.T) updateOptions {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /tpm/nonce/{wwid}", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, testWWID, r.PathValue("wwid"))
		s.gotQuery = map[string]string{"assetkey": r.URL.Query().Get("assetkey"), "uuid": r.URL.Query().Get("uuid")}
		s.nonce = []byte("0123456789abcdef0123456789abcdef")
		_ = json.NewEncoder(w).Encode(tpm.NonceResponse{Nonce: s.nonce, TTL: 60})
	})
	mux.HandleFunc("POST /tpm/quote/{wwid}", func(w http.ResponseWriter, r *http.Request) {
		var ev tpm.Evidence
		require.NoError(t, json.NewDecoder(r.Body).Decode(&ev))
		assert.Equal(t, s.nonce, ev.Nonce, "evidence is quoted over the server's nonce")
		switch s.quoteCode {
		case "":
		case tpm.CodeBaselineChanged, tpm.CodeEKChanged:
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(tpm.QuoteResponse{Error: s.quoteCode, Credential: &attest.EncryptedCredential{Secret: testSecret}})
			return
		default:
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(tpm.ErrorResponse{Error: s.quoteCode})
			return
		}
		_ = json.NewEncoder(w).Encode(tpm.QuoteResponse{Credential: &attest.EncryptedCredential{Secret: testSecret}})
	})
	mux.HandleFunc("POST /tpm/confirm/{wwid}", func(w http.ResponseWriter, r *http.Request) {
		secret, _ := hex.DecodeString(r.Header.Get(tpm.SecretHeader))
		s.confirmed = append(s.confirmed, secret)
		if !bytes.Equal(secret, testSecret) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(tpm.ConfirmResponse{Candidate: "0123456789abcdef"})
	})
	mux.HandleFunc("GET /runtime/{wwid}", func(w http.ResponseWriter, r *http.Request) {
		secret, _ := hex.DecodeString(r.Header.Get(tpm.SecretHeader))
		s.secrets = append(s.secrets, secret)
		if !bytes.Equal(secret, testSecret) {
			w.Header().Set(tpm.RequiredHeader, "required")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if !s.runtimeOK {
			w.WriteHeader(http.StatusNotFound)
		}
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)

	savedClient, savedDelay := webclient, statusRetryDelay
	webclient, statusRetryDelay = srv.Client(), 0
	t.Cleanup(func() { webclient, statusRetryDelay = savedClient, savedDelay })

	host, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	require.NoError(t, err)
	p, err := strconv.Atoi(port)
	require.NoError(t, err)
	return updateOptions{scheme: "https", ipaddr: host, port: p, wwid: testWWID, tag: "rack1", localUUID: uuid.New()}
}

func TestAttestTPM(t *testing.T) {
	s := &fakeServer{}
	o := s.start(t)

	secret, err := attestTPM(webclient, o, openFake)
	require.NoError(t, err)
	assert.Equal(t, testSecret, secret)
	assert.Equal(t, "rack1", s.gotQuery["assetkey"], "TPM requests carry the asset key")
	assert.Equal(t, o.localUUID.String(), s.gotQuery["uuid"])
}

func TestAttestTPMRejected(t *testing.T) {
	s := &fakeServer{quoteCode: tpm.CodeNoKernel}
	o := s.start(t)

	_, err := attestTPM(webclient, o, openFake)
	require.Error(t, err)
	assert.Contains(t, err.Error(), string(tpm.CodeNoKernel), "the server's reason is reported")
}

// When the server reports a changed baseline, the node proves it holds the
// TPM by decrypting the issued credential, and the change is recorded.
func TestAttestTPMChange(t *testing.T) {
	s := &fakeServer{quoteCode: tpm.CodeBaselineChanged}
	o := s.start(t)

	_, err := attestTPM(webclient, o, openFake)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "baseline_changed")
	assert.Contains(t, err.Error(), "candidate 0123456789abcdef, awaiting approval")
	assert.Equal(t, [][]byte{testSecret}, s.confirmed, "the decrypted secret confirms the change")
	assert.Empty(t, s.secrets, "the change's secret is not used for the runtime overlay")
}

func TestUpdateSystemTPM(t *testing.T) {
	s := &fakeServer{}
	o := s.start(t)

	assert.ErrorIs(t, updateSystem(o), errTPMRequired, "a 401 with the TPM header asks for attestation")
	assert.Empty(t, s.secrets[0], "no secret is sent before attestation")

	o.secret = testSecret
	assert.NoError(t, updateSystem(o), "other refusals are not TPM errors")
	assert.Equal(t, testSecret, s.secrets[1], "the secret is sent in a header")
}

func TestSecretFile(t *testing.T) {
	name := filepath.Join(t.TempDir(), "warewulf", "secret")
	assert.Nil(t, loadSecret(name))

	require.NoError(t, saveSecret(name, testSecret))
	assert.Equal(t, testSecret, loadSecret(name))
	info, err := os.Stat(name)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	require.NoError(t, os.WriteFile(name, []byte("not hex"), 0o600))
	assert.Nil(t, loadSecret(name), "a malformed secret is ignored")
}
