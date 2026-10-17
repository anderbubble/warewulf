package wwclient

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/google/go-attestation/attest"
	"github.com/warewulf/warewulf/internal/pkg/tpm"
	"github.com/warewulf/warewulf/internal/pkg/wwlog"
)

// errTPMRequired is returned by updateSystem when the server requires TPM
// attestation before it will send the runtime overlay.
var errTPMRequired = errors.New("server requires TPM attestation")

// attestor produces attestation evidence and decrypts credentials with the
// node's TPM.
type attestor interface {
	Evidence(nonce []byte) (*tpm.Evidence, error)
	Activate(*attest.EncryptedCredential) ([]byte, error)
	Close() error
}

// attestTPM proves the node's boot to the server and returns the runtime
// secret the server issues for it. If the server reports that the TPM or its
// baseline changed, attestTPM proves to it that this TPM decrypted the issued
// credential, so the change is recorded for an admin to approve, and returns
// an error saying so.
func attestTPM(client *http.Client, o updateOptions, open func() (attestor, error)) ([]byte, error) {
	// Open the TPM (which may create an AK, taking a few seconds) before
	// requesting the short-lived nonce.
	a, err := open()
	if err != nil {
		return nil, fmt.Errorf("opening TPM: %w", err)
	}
	defer func() { _ = a.Close() }()

	var nr tpm.NonceResponse
	if err := postJSON(client, o.url("tpm/nonce/"+o.wwid), nil, nil, &nr); err != nil {
		return nil, fmt.Errorf("requesting nonce: %w", err)
	}
	ev, err := a.Evidence(nr.Nonce)
	if err != nil {
		return nil, fmt.Errorf("quoting: %w", err)
	}
	var qr tpm.QuoteResponse
	err = postJSON(client, o.url("tpm/quote/"+o.wwid), nil, ev, &qr)
	var se *statusError
	if errors.As(err, &se) && se.code == http.StatusConflict && json.Unmarshal(se.body, &qr) == nil && qr.Credential != nil {
		return nil, confirmChange(client, o, a, &qr)
	}
	if err != nil {
		return nil, fmt.Errorf("sending quote: %w", err)
	}
	if qr.Credential == nil {
		return nil, errors.New("server sent no credential")
	}
	secret, err := a.Activate(qr.Credential)
	if err != nil {
		return nil, fmt.Errorf("activating credential: %w", err)
	}
	return secret, nil
}

// confirmChange decrypts the credential issued for a changed TPM or baseline
// and presents the secret, so the server records the change for approval.
func confirmChange(client *http.Client, o updateOptions, a attestor, qr *tpm.QuoteResponse) error {
	secret, err := a.Activate(qr.Credential)
	if err != nil {
		return fmt.Errorf("TPM %s: activating credential: %w", qr.Error, err)
	}
	var cr tpm.ConfirmResponse
	if err := postJSON(client, o.url("tpm/confirm/"+o.wwid), secret, nil, &cr); err != nil {
		return fmt.Errorf("TPM %s: confirming change: %w", qr.Error, err)
	}
	return fmt.Errorf("TPM %s: recorded on the server as candidate %s, awaiting approval", qr.Error, cr.Candidate)
}

// statusError is a response other than 200.
type statusError struct {
	status string
	code   int
	reason tpm.Code
	body   []byte
}

func (e *statusError) Error() string {
	if e.reason != "" {
		return fmt.Sprintf("server returned %s: %s", e.status, e.reason)
	}
	return "server returned " + e.status
}

// postJSON posts in as JSON (no body if nil), with secret in the TPM secret
// header if given, and decodes a 200 response into out.
func postJSON(client *http.Client, url string, secret []byte, in, out any) error {
	var body io.Reader = http.NoBody
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(http.MethodPost, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if secret != nil {
		req.Header.Set(tpm.SecretHeader, hex.EncodeToString(secret))
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		se := &statusError{status: resp.Status, code: resp.StatusCode, body: data}
		var e tpm.ErrorResponse
		if json.Unmarshal(data, &e) == nil {
			se.reason = e.Error
		}
		return se
	}
	return json.Unmarshal(data, out)
}

// loadSecret returns the saved runtime secret, if any.
func loadSecret(name string) []byte {
	data, err := os.ReadFile(name)
	if err != nil {
		return nil
	}
	secret, err := hex.DecodeString(strings.TrimSpace(string(data)))
	if err != nil {
		wwlog.Warn("ignoring malformed TPM secret in %s", name)
		return nil
	}
	return secret
}

// saveSecret writes secret, hex encoded, readable only by root.
func saveSecret(name string, secret []byte) error {
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(name), "."+filepath.Base(name)+".*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(hex.EncodeToString(secret)); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), name)
}

// tpmAttestor uses the node's TPM through go-attestation.
type tpmAttestor struct {
	t  *attest.TPM
	ek attest.EK
	ak *attest.AK
}

// openTPMAttestor returns a function that opens the node's TPM, keeping its
// attestation key at akPath.
func openTPMAttestor(akPath string) func() (attestor, error) {
	return func() (attestor, error) {
		t, err := attest.OpenTPM(nil)
		if err != nil {
			return nil, err
		}
		a := &tpmAttestor{t: t}
		eks, err := t.EKs()
		if err != nil {
			_ = t.Close()
			return nil, fmt.Errorf("reading endorsement keys: %w", err)
		}
		if len(eks) == 0 {
			_ = t.Close()
			return nil, errors.New("TPM has no endorsement key")
		}
		// Prefer the RSA EK, so the identity the server enrolls is the same
		// on every boot.
		a.ek = eks[0]
		if i := slices.IndexFunc(eks, func(ek attest.EK) bool { _, ok := ek.Public.(*rsa.PublicKey); return ok }); i >= 0 {
			a.ek = eks[i]
		}
		if a.ak, err = loadOrCreateAK(t, akPath); err != nil {
			_ = t.Close()
			return nil, err
		}
		return a, nil
	}
}

func loadOrCreateAK(t *attest.TPM, akPath string) (*attest.AK, error) {
	if blob, err := os.ReadFile(akPath); err == nil {
		ak, err := t.LoadAK(blob)
		if err == nil {
			wwlog.Debug("loaded attestation key from %s", akPath)
			return ak, nil
		}
		wwlog.Warn("creating a new attestation key: loading %s: %s", akPath, err)
	}

	ak, err := t.NewAK(nil)
	if err != nil {
		return nil, fmt.Errorf("creating attestation key: %w", err)
	}
	if blob, err := ak.Marshal(); err != nil {
		wwlog.Warn("not saving attestation key: %s", err)
	} else if err := os.MkdirAll(filepath.Dir(akPath), 0o700); err != nil {
		wwlog.Warn("not saving attestation key: %s", err)
	} else if err := os.WriteFile(akPath, blob, 0o600); err != nil {
		wwlog.Warn("not saving attestation key: %s", err)
	}
	return ak, nil
}

func (a *tpmAttestor) Evidence(nonce []byte) (*tpm.Evidence, error) {
	q, err := a.ak.QuotePCRs(a.t, nonce, attest.HashSHA256, tpm.QuotedPCRs)
	if err != nil {
		return nil, err
	}
	pcrs, err := a.t.PCRs(attest.HashSHA256)
	if err != nil {
		return nil, fmt.Errorf("reading PCRs: %w", err)
	}
	values := map[int][]byte{}
	for _, p := range pcrs {
		if slices.Contains(tpm.QuotedPCRs, p.Index) {
			values[p.Index] = p.Digest
		}
	}
	eventLog, err := a.t.MeasurementLog()
	if err != nil {
		return nil, fmt.Errorf("reading event log: %w", err)
	}
	ekPub, err := x509.MarshalPKIXPublicKey(a.ek.Public)
	if err != nil {
		return nil, fmt.Errorf("encoding endorsement key: %w", err)
	}
	var ekCert []byte
	if a.ek.Certificate != nil {
		ekCert = a.ek.Certificate.Raw
	}
	p := a.ak.AttestationParameters()
	return &tpm.Evidence{
		Nonce:  nonce,
		EKPub:  ekPub,
		EKCert: ekCert,
		AK: tpm.AKParameters{
			Public:            p.Public,
			CreateData:        p.CreateData,
			CreateAttestation: p.CreateAttestation,
			CreateSignature:   p.CreateSignature,
		},
		Quote:     q.Quote,
		Signature: q.Signature,
		PCRs:      values,
		EventLog:  eventLog,
	}, nil
}

func (a *tpmAttestor) Activate(c *attest.EncryptedCredential) ([]byte, error) {
	return a.ak.ActivateCredentialWithEK(a.t, *c, a.ek)
}

func (a *tpmAttestor) Close() error {
	_ = a.ak.Close(a.t)
	return a.t.Close()
}
