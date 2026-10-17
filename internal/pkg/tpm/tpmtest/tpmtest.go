// Package tpmtest is a software stand-in for a TPM 2.0, for tests. It
// produces evidence that go-attestation verifies: a restricted RSA AK with a
// creation attestation, quotes over SHA-256 PCRs, and a crypto-agile event
// log that replays to those PCRs. It does not implement credential activation.
package tpmtest

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
	"slices"
	"time"

	"github.com/google/go-tpm/legacy/tpm2"
	"github.com/warewulf/warewulf/internal/pkg/tpm"
)

// Event types used by boot loaders.
const (
	EvNoAction = 0x03
	EvIPL      = 0x0D
)

const numPCRs = 24

// TPM is a software TPM.
type TPM struct {
	EK *rsa.PrivateKey
	AK *rsa.PrivateKey

	akPublic tpm2.Public
	ak       tpm.AKParameters
	pcrs     [numPCRs][]byte
	log      *EventLog
}

// New returns a TPM with fresh EK and AK keys, zeroed PCRs and an empty event
// log.
func New() (*TPM, error) {
	ek, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	t := &TPM{EK: ek}
	for i := range t.pcrs {
		t.pcrs[i] = make([]byte, sha256.Size)
	}
	t.log = NewEventLog()
	if err := t.NewAK(); err != nil {
		return nil, err
	}
	return t, nil
}

// MustNew is New for tests that cannot proceed without a TPM.
func MustNew() *TPM {
	t, err := New()
	if err != nil {
		panic(err)
	}
	return t
}

// NewAK replaces the AK, as happens when a node is re-provisioned.
func (t *TPM) NewAK() error {
	ak, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	t.AK = ak
	t.akPublic = tpm2.Public{
		Type:    tpm2.AlgRSA,
		NameAlg: tpm2.AlgSHA256,
		Attributes: tpm2.FlagSign | tpm2.FlagRestricted | tpm2.FlagFixedTPM |
			tpm2.FlagFixedParent | tpm2.FlagSensitiveDataOrigin | tpm2.FlagUserWithAuth,
		RSAParameters: &tpm2.RSAParams{
			Sign:       &tpm2.SigScheme{Alg: tpm2.AlgRSASSA, Hash: tpm2.AlgSHA256},
			KeyBits:    2048,
			ModulusRaw: ak.N.Bytes(),
		},
	}
	pub, err := t.akPublic.Encode()
	if err != nil {
		return err
	}
	name, err := t.akPublic.Name()
	if err != nil {
		return err
	}
	parent := tpm2.Name{Digest: &tpm2.HashValue{Alg: tpm2.AlgSHA256, Value: make([]byte, sha256.Size)}}
	createData, err := (&tpm2.CreationData{
		PCRSelection:        tpm2.PCRSelection{Hash: tpm2.AlgSHA256},
		ParentNameAlg:       tpm2.AlgSHA256,
		ParentName:          parent,
		ParentQualifiedName: parent,
	}).EncodeCreationData()
	if err != nil {
		return err
	}
	createDigest := sha256.Sum256(createData)
	att, err := tpm2.AttestationData{
		Magic:                0xff544347,
		Type:                 tpm2.TagAttestCreation,
		QualifiedSigner:      parent,
		AttestedCreationInfo: &tpm2.CreationInfo{Name: name, OpaqueDigest: createDigest[:]},
	}.Encode()
	if err != nil {
		return err
	}
	sig, err := t.sign(att)
	if err != nil {
		return err
	}
	t.ak = tpm.AKParameters{Public: pub, CreateData: createData, CreateAttestation: att, CreateSignature: sig}
	return nil
}

// Extend measures data into pcr, as a boot loader measuring a file would,
// and logs it.
func (t *TPM) Extend(pcr int, eventType uint32, data []byte) {
	digest := sha256.Sum256(data)
	t.ExtendDigest(pcr, eventType, digest[:], data)
}

// ExtendDigest extends pcr with digest and logs eventData with it.
func (t *TPM) ExtendDigest(pcr int, eventType uint32, digest, eventData []byte) {
	h := sha256.New()
	h.Write(t.pcrs[pcr])
	h.Write(digest)
	t.pcrs[pcr] = h.Sum(nil)
	t.log.Add(pcr, eventType, digest, eventData)
}

// ExtendSilently changes pcr without logging it, as tampering would.
func (t *TPM) ExtendSilently(pcr int, digest []byte) {
	h := sha256.New()
	h.Write(t.pcrs[pcr])
	h.Write(digest)
	t.pcrs[pcr] = h.Sum(nil)
}

// PCR returns the current value of pcr.
func (t *TPM) PCR(pcr int) []byte { return slices.Clone(t.pcrs[pcr]) }

// EventLog returns the current measurement log.
func (t *TPM) EventLog() []byte { return t.log.Bytes() }

// EKPub returns the EK public key, PKIX DER encoded.
func (t *TPM) EKPub() []byte {
	b, err := x509.MarshalPKIXPublicKey(&t.EK.PublicKey)
	if err != nil {
		panic(err)
	}
	return b
}

// EKCert returns a self-signed certificate for pub.
func EKCert(pub crypto.PublicKey) []byte {
	ca, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "tpmtest EK"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, ca)
	if err != nil {
		panic(err)
	}
	return der
}

// Evidence quotes pcrs (tpm.QuotedPCRs if nil) over nonce and returns the
// evidence a node would send.
func (t *TPM) Evidence(nonce []byte, pcrs []int) (*tpm.Evidence, error) {
	if pcrs == nil {
		pcrs = tpm.QuotedPCRs
	}
	sel := slices.Sorted(slices.Values(pcrs))
	values := map[int][]byte{}
	for _, i := range sel {
		values[i] = slices.Clone(t.pcrs[i])
	}
	quote, sig, err := t.QuoteOver(nonce, tpm2.AlgSHA256, sel)
	if err != nil {
		return nil, err
	}
	return &tpm.Evidence{
		Nonce:     nonce,
		EKPub:     t.EKPub(),
		AK:        t.ak,
		Quote:     quote,
		Signature: sig,
		PCRs:      values,
		EventLog:  t.EventLog(),
	}, nil
}

// QuoteOver signs a quote over nonce selecting the PCRs sel in the bank alg,
// so tests can quote something other than the PCRs they report. The digest is
// over this TPM's SHA-256 PCR values, so it is only meaningful for an empty
// selection or the SHA-256 bank.
func (t *TPM) QuoteOver(nonce []byte, alg tpm2.Algorithm, sel []int) (quote, sig []byte, err error) {
	sel = slices.Sorted(slices.Values(sel))
	h := sha256.New()
	for _, i := range sel {
		h.Write(t.pcrs[i])
	}
	quote, err = tpm2.AttestationData{
		Magic:             0xff544347,
		Type:              tpm2.TagAttestQuote,
		QualifiedSigner:   tpm2.Name{Digest: &tpm2.HashValue{Alg: tpm2.AlgSHA256, Value: make([]byte, sha256.Size)}},
		ExtraData:         nonce,
		AttestedQuoteInfo: &tpm2.QuoteInfo{PCRSelection: tpm2.PCRSelection{Hash: alg, PCRs: sel}, PCRDigest: h.Sum(nil)},
	}.Encode()
	if err != nil {
		return nil, nil, fmt.Errorf("encoding quote: %w", err)
	}
	sig, err = t.sign(quote)
	return quote, sig, err
}

func (t *TPM) sign(msg []byte) ([]byte, error) {
	digest := sha256.Sum256(msg)
	s, err := rsa.SignPKCS1v15(rand.Reader, t.AK, crypto.SHA256, digest[:])
	if err != nil {
		return nil, err
	}
	return tpm2.Signature{Alg: tpm2.AlgRSASSA, RSA: &tpm2.SignatureRSA{HashAlg: tpm2.AlgSHA256, Signature: s}}.Encode()
}

// EventLog builds a crypto-agile measurement log with SHA-256 digests.
type EventLog struct {
	buf bytes.Buffer
}

// NewEventLog returns a log holding only its header event.
func NewEventLog() *EventLog {
	l := &EventLog{}
	var spec bytes.Buffer
	spec.WriteString("Spec ID Event03\x00")
	_ = binary.Write(&spec, binary.LittleEndian, uint32(0)) // platform class
	spec.Write([]byte{0, 2, 0, 2})                          // minor, major, errata, uintn size
	_ = binary.Write(&spec, binary.LittleEndian, uint32(1)) // algorithm count
	_ = binary.Write(&spec, binary.LittleEndian, uint16(tpm2.AlgSHA256))
	_ = binary.Write(&spec, binary.LittleEndian, uint16(sha256.Size))
	spec.WriteByte(0) // vendor info size

	// The header event uses the SHA-1 log format.
	_ = binary.Write(&l.buf, binary.LittleEndian, uint32(0))          // PCR index
	_ = binary.Write(&l.buf, binary.LittleEndian, uint32(EvNoAction)) // event type
	l.buf.Write(make([]byte, 20))                                     // SHA-1 digest
	_ = binary.Write(&l.buf, binary.LittleEndian, uint32(spec.Len()))
	l.buf.Write(spec.Bytes())
	return l
}

// Add logs an event that extended pcr with digest.
func (l *EventLog) Add(pcr int, eventType uint32, digest, eventData []byte) {
	_ = binary.Write(&l.buf, binary.LittleEndian, uint32(pcr))
	_ = binary.Write(&l.buf, binary.LittleEndian, eventType)
	_ = binary.Write(&l.buf, binary.LittleEndian, uint32(1)) // digest count
	_ = binary.Write(&l.buf, binary.LittleEndian, uint16(tpm2.AlgSHA256))
	l.buf.Write(digest)
	_ = binary.Write(&l.buf, binary.LittleEndian, uint32(len(eventData)))
	l.buf.Write(eventData)
}

// Bytes returns the log.
func (l *EventLog) Bytes() []byte { return slices.Clone(l.buf.Bytes()) }

// Attest records a GRUB boot of a kernel and an attestation of t for node in
// store, as warewulfd and a node that proves it holds the TPM would: the
// issued secret is used, which commits an enrollment, or, if the TPM or
// baseline changed, confirms the candidate and returns its *tpm.Error. It
// returns the issued secret.
func (t *TPM) Attest(store *tpm.Store, node string) ([]byte, error) {
	kernel := []byte("kernel for " + node)
	if bytes.Equal(t.PCR(0), make([]byte, sha256.Size)) {
		t.Extend(0, 0x80000008, []byte("firmware"))
	}
	t.Extend(tpm.PCRBootFiles, EvIPL, kernel)
	sum := sha256.Sum256(kernel)
	sent := tpm.SentFile{Path: "vmlinuz", Stage: tpm.StageKernel, Digest: hex.EncodeToString(sum[:])}

	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ev, err := t.Evidence(nonce, nil)
	if err != nil {
		return nil, err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	err = store.Update(node, func(st *tpm.State) error {
		st.StartBoot(time.Now())
		st.RecordSent(sent)
		v, err := tpm.VerifyEvidence(ev, nonce, st.SentFiles())
		if err != nil {
			st.Record(err, time.Now())
			return err
		}
		enroll, candidate, err := st.Evaluate(ev, v, time.Now())
		if candidate != nil {
			st.AddConfirmation(secret, candidate, time.Now())
			st.ConfirmCandidate(secret, time.Now())
		} else {
			st.AddPending(secret, ev, v, st.SentFiles(), enroll, time.Now())
			st.CheckSecret(secret, time.Now())
		}
		return err
	})
	return secret, err
}
