package tpm

import (
	"bytes"
	"crypto"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"

	"github.com/google/go-attestation/attest"
)

// PCRBootFiles is the PCR into which GRUB measures each file it loads.
const PCRBootFiles = 9

// BaselinePCRs are compared against the values recorded at enrollment: firmware
// code (0), option ROM code (2), boot manager code (4) and Secure Boot policy
// (7). They change only with firmware, hardware or boot loader changes, not
// with image or overlay changes.
var BaselinePCRs = []int{0, 2, 4, 7}

// QuotedPCRs are the PCRs a node quotes. PCR 10 and above are left out
// because IMA and the running OS extend them continuously, which would race
// with reading their values for the quote.
var QuotedPCRs = []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}

// requiredPCRs must be present in every quote.
var requiredPCRs = append(slices.Clone(BaselinePCRs), PCRBootFiles)

// Stages of boot files the server records for a node. StageKernel must be
// among the measured files for an attestation to pass.
const (
	StageKernel = "kernel"
)

// Evidence is what a node sends to the server to attest its boot.
type Evidence struct {
	// Nonce is the server-issued nonce the quote was made over.
	Nonce []byte `json:"nonce"`
	// EKPub is the endorsement key's public key, PKIX DER encoded.
	EKPub []byte `json:"ek_pub"`
	// EKCert is the endorsement key certificate, DER encoded, if the TPM has one.
	EKCert []byte `json:"ek_cert,omitempty"`
	// AK holds the attestation key's public area and its creation attestation.
	AK AKParameters `json:"ak"`
	// Quote and Signature are the TPM2_Quote output.
	Quote     []byte `json:"quote"`
	Signature []byte `json:"signature"`
	// PCRs are the SHA-256 PCR values covered by the quote, by index.
	PCRs map[int][]byte `json:"pcrs"`
	// EventLog is the firmware/boot loader measurement log.
	EventLog []byte `json:"event_log"`
}

// AKParameters mirrors attest.AttestationParameters for a TPM 2.0 AK.
type AKParameters struct {
	Public            []byte `json:"public"`
	CreateData        []byte `json:"create_data"`
	CreateAttestation []byte `json:"create_attestation"`
	CreateSignature   []byte `json:"create_signature"`
}

// Attestation converts p to go-attestation's form.
func (p AKParameters) Attestation() attest.AttestationParameters {
	return attest.AttestationParameters{
		Public:            p.Public,
		CreateData:        p.CreateData,
		CreateAttestation: p.CreateAttestation,
		CreateSignature:   p.CreateSignature,
	}
}

// SentFile records a file the server sent to a node, so measurements of it
// can be recognized.
type SentFile struct {
	Path   string `json:"path"`
	Stage  string `json:"stage"`
	Digest string `json:"digest"` // hex SHA-256
}

// Code identifies why an attestation was rejected. Codes are returned to
// nodes and shown to admins, so they are stable strings.
type Code string

const (
	CodeBadEvidence      Code = "bad_evidence"
	CodeQuoteInvalid     Code = "quote_invalid"
	CodeEventLogReplay   Code = "eventlog_replay"
	CodeNoBootFiles      Code = "no_boot_files"
	CodeBootFilesUnknown Code = "boot_files_unknown"
	CodeNoKernel         Code = "no_kernel_measurement"
	CodeEKCertMismatch   Code = "ek_cert_mismatch"
	CodeEKChanged        Code = "ek_changed"
	CodeBaselineChanged  Code = "baseline_changed"
)

// Error is a rejected attestation.
type Error struct {
	Code Code
	Err  error
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %v", e.Code, e.Err) }
func (e *Error) Unwrap() error { return e.Err }

func newError(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Err: fmt.Errorf(format, args...)}
}

// CodeOf returns the Code of err, or "" if err is not an *Error.
func CodeOf(err error) Code {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

// Verified is the result of a successful VerifyEvidence.
type Verified struct {
	EKPub         crypto.PublicKey
	EKFingerprint string
	AKFingerprint string
	PCRs          map[int][]byte
	// Events are the event log entries proven by replay against the quoted PCRs.
	Events []attest.Event
}

// Fingerprint returns the hex SHA-256 of b.
func Fingerprint(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// VerifyEvidence checks that ev is a fresh quote over nonce from a genuine TPM
// AK, that the event log replays to the quoted PCRs, that every file measured
// into PCRBootFiles is one the server sent (and includes a kernel), and that
// any EK certificate matches the EK. It does not check enrollment; see
// DiffBaseline.
func VerifyEvidence(ev *Evidence, nonce []byte, sent []SentFile) (*Verified, error) {
	if ev == nil {
		return nil, newError(CodeBadEvidence, "no evidence")
	}
	ekPub, err := x509.ParsePKIXPublicKey(ev.EKPub)
	if err != nil {
		return nil, newError(CodeBadEvidence, "parsing EK public key: %v", err)
	}
	for _, i := range requiredPCRs {
		if _, ok := ev.PCRs[i]; !ok {
			return nil, newError(CodeBadEvidence, "PCR %d missing from quote", i)
		}
	}

	// The AK must be a restricted, TPM-resident signing key.
	if err := (&attest.ActivationParameters{EK: ekPub, AK: ev.AK.Attestation()}).CheckAKParameters(); err != nil {
		return nil, newError(CodeQuoteInvalid, "attestation key: %v", err)
	}
	akPub, err := attest.ParseAKPublic(ev.AK.Public)
	if err != nil {
		return nil, newError(CodeQuoteInvalid, "parsing AK public key: %v", err)
	}
	pcrs := attestPCRs(ev.PCRs)
	if err := akPub.Verify(attest.Quote{Quote: ev.Quote, Signature: ev.Signature}, pcrs, nonce); err != nil {
		return nil, newError(CodeQuoteInvalid, "%v", err)
	}
	// Verify checks only the PCRs in the quote's own bank: a quote over an
	// empty SHA-1 selection would leave every SHA-256 value unchecked.
	for _, p := range pcrs {
		if !p.QuoteVerified() {
			return nil, newError(CodeQuoteInvalid, "PCR %d is not covered by the quote", p.Index)
		}
	}

	el, err := attest.ParseEventLog(ev.EventLog)
	if err != nil {
		return nil, newError(CodeEventLogReplay, "parsing event log: %v", err)
	}
	events, err := el.Verify(pcrs)
	if err != nil {
		return nil, newError(CodeEventLogReplay, "%v", err)
	}
	if err := VerifyBootFiles(events, sent); err != nil {
		return nil, err
	}

	if len(ev.EKCert) > 0 {
		cert, err := attest.ParseEKCertificate(ev.EKCert)
		if err != nil {
			return nil, newError(CodeEKCertMismatch, "parsing EK certificate: %v", err)
		}
		certPub, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
		if err != nil || !bytes.Equal(certPub, ev.EKPub) {
			return nil, newError(CodeEKCertMismatch, "EK certificate is for a different key")
		}
	}

	return &Verified{
		EKPub:         ekPub,
		EKFingerprint: Fingerprint(ev.EKPub),
		AKFingerprint: Fingerprint(ev.AK.Public),
		PCRs:          ev.PCRs,
		Events:        events,
	}, nil
}

// VerifyBootFiles checks the verified events in PCRBootFiles: there must be at
// least one, each must be a file the server sent, and at least one must be a
// kernel. A boot that GRUB did not perform (iPXE, local disk) has no such
// events and fails.
func VerifyBootFiles(events []attest.Event, sent []SentFile) error {
	return checkBootFiles(events, sent, true)
}

func checkBootFiles(events []attest.Event, sent []SentFile, requireKernel bool) error {
	stages := map[string][]string{}
	for _, f := range sent {
		stages[f.Digest] = append(stages[f.Digest], f.Stage)
	}
	count, kernel := 0, false
	for _, e := range events {
		if e.Index != PCRBootFiles {
			continue
		}
		count++
		digest := hex.EncodeToString(e.Digest)
		st, ok := stages[digest]
		if !ok {
			return newError(CodeBootFilesUnknown, "measured file %s (%s) was not sent by this server", digest, FormatEventData(e))
		}
		if slices.Contains(st, StageKernel) {
			kernel = true
		}
	}
	if count == 0 {
		return newError(CodeNoBootFiles, "no boot files measured in PCR %d", PCRBootFiles)
	}
	if requireKernel && !kernel {
		return newError(CodeNoKernel, "no kernel sent by this server was measured in PCR %d", PCRBootFiles)
	}
	return nil
}

// DiffBaseline returns the BaselinePCRs whose values differ between enrolled
// and current.
func DiffBaseline(enrolled, current map[int][]byte) []int {
	var diff []int
	for _, i := range BaselinePCRs {
		if !bytes.Equal(enrolled[i], current[i]) {
			diff = append(diff, i)
		}
	}
	return diff
}

// BaselineOf returns the BaselinePCRs values from pcrs.
func BaselineOf(pcrs map[int][]byte) map[int][]byte {
	out := make(map[int][]byte, len(BaselinePCRs))
	for _, i := range BaselinePCRs {
		out[i] = pcrs[i]
	}
	return out
}

func attestPCRs(in map[int][]byte) []attest.PCR {
	pcrs := make([]attest.PCR, 0, len(in))
	for i, digest := range in {
		pcrs = append(pcrs, attest.PCR{Index: i, Digest: digest, DigestAlg: crypto.SHA256})
	}
	slices.SortFunc(pcrs, func(a, b attest.PCR) int { return a.Index - b.Index })
	return pcrs
}
