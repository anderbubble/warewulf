package tpm

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// StateVersion is the current State format.
const StateVersion = 2

const (
	// MaxPending is how many issued-but-unused secrets a node may have.
	MaxPending = 4
	// PendingTTL is how long an issued secret stays usable before first use.
	PendingTTL = 10 * time.Minute
)

// ErrNoCandidate is returned by Approve when there is nothing to approve.
var ErrNoCandidate = errors.New("no candidate to approve")

// State is what the server keeps about a node's TPM.
type State struct {
	Version int    `json:"version"`
	Node    string `json:"node"`

	// Enrolled is the TPM identity and baseline accepted for the node.
	Enrolled *Enrollment `json:"enrolled,omitempty"`
	// Candidate is a differing identity or baseline awaiting approval.
	Candidate *Candidate `json:"candidate,omitempty"`

	// Current is the secret the node is using for its runtime overlay.
	Current *Secret `json:"current,omitempty"`
	// Pending are secrets issued but not yet used. The first one the node
	// presents becomes Current, so issuing one never invalidates Current.
	Pending []Secret `json:"pending,omitempty"`
	// Confirmations are secrets issued for a changed TPM or baseline, which
	// the node presents to have the change recorded as a Candidate. They are
	// kept apart from Pending so change attempts can't evict runtime secrets.
	Confirmations []Secret `json:"confirmations,omitempty"`

	LastAttempt *Attempt `json:"last_attempt,omitempty"`

	// Boot holds the files sent during this boot [0] and the previous one [1].
	Boot [2]BootSession `json:"boot"`
}

// Enrollment is an accepted TPM identity and baseline.
type Enrollment struct {
	EKPub         []byte         `json:"ek_pub"`
	EKCert        []byte         `json:"ek_cert,omitempty"`
	EKFingerprint string         `json:"ek_fingerprint"`
	Baseline      map[int][]byte `json:"baseline"`
	At            time.Time      `json:"at"`
}

// Candidate is attestation evidence that verified but differs from the
// enrollment.
type Candidate struct {
	Reason Code `json:"reason"`
	Enrollment
	// Changed lists the baseline PCRs that differ, for baseline_changed.
	Changed []int `json:"changed,omitempty"`
}

// ID identifies the candidate's TPM and baseline, so an admin can approve
// exactly the candidate they inspected: a later rejected attestation replaces
// the candidate, and changes the ID if its TPM or baseline differs.
func (c *Candidate) ID() string {
	h := sha256.New()
	h.Write([]byte(c.EKFingerprint))
	for _, i := range BaselinePCRs {
		h.Write([]byte{byte(i)})
		h.Write(c.Baseline[i])
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// Secret is an issued runtime-overlay secret, stored only as a hash.
type Secret struct {
	Hash          string     `json:"hash"`
	AKFingerprint string     `json:"ak_fingerprint"`
	IssuedAt      time.Time  `json:"issued_at"`
	UsedAt        *time.Time `json:"used_at,omitempty"`
	// Evidence and Sent are what the secret was issued for, kept so admins
	// can re-verify it offline.
	Evidence *Evidence  `json:"evidence,omitempty"`
	Sent     []SentFile `json:"sent,omitempty"`
	// Enroll and Candidate are what the secret's first use commits.
	Enroll    *Enrollment `json:"enroll,omitempty"`
	Candidate *Candidate  `json:"candidate,omitempty"`
}

// Attempt is the outcome of the last attestation.
type Attempt struct {
	At     time.Time `json:"at"`
	Code   Code      `json:"code,omitempty"` // empty on success
	Detail string    `json:"detail,omitempty"`
}

// OK reports whether the attempt succeeded.
func (a *Attempt) OK() bool { return a != nil && a.Code == "" }

// BootSession is the set of files sent to a node during one boot.
type BootSession struct {
	Started time.Time  `json:"started,omitzero"`
	Sent    []SentFile `json:"sent,omitempty"`
}

// HashSecret returns the stored form of secret.
func HashSecret(secret []byte) string {
	sum := sha256.Sum256(secret)
	return hex.EncodeToString(sum[:])
}

// StartBoot begins a new boot session, keeping the previous one.
func (s *State) StartBoot(now time.Time) {
	s.Boot[1] = s.Boot[0]
	s.Boot[0] = BootSession{Started: now}
}

// RecordSent adds f to the current boot session.
func (s *State) RecordSent(f SentFile) {
	for _, have := range s.Boot[0].Sent {
		if have == f {
			return
		}
	}
	s.Boot[0].Sent = append(s.Boot[0].Sent, f)
}

// SentFiles returns the files sent in the current and previous boot.
func (s *State) SentFiles() []SentFile {
	return append(append([]SentFile{}, s.Boot[0].Sent...), s.Boot[1].Sent...)
}

// Evaluate compares verified evidence with the enrollment, without changing
// anything. It returns the enrollment to make if the node is not enrolled, or
// the candidate to record, with an *Error, if its TPM or baseline differs.
//
// Nothing is committed here: the evidence does not prove the AK is in the TPM
// holding the claimed EK. The node proves that by decrypting the credential
// issued for it, so CheckSecret commits the enrollment and ConfirmCandidate
// the candidate.
func (s *State) Evaluate(ev *Evidence, v *Verified, now time.Time) (*Enrollment, *Candidate, error) {
	seen := Enrollment{
		EKPub:         ev.EKPub,
		EKCert:        ev.EKCert,
		EKFingerprint: v.EKFingerprint,
		Baseline:      BaselineOf(v.PCRs),
		At:            now,
	}
	if s.Enrolled == nil {
		return &seen, nil, nil
	}
	if s.Enrolled.EKFingerprint != seen.EKFingerprint {
		return nil, &Candidate{Reason: CodeEKChanged, Enrollment: seen},
			newError(CodeEKChanged, "TPM endorsement key differs from enrollment")
	}
	if changed := DiffBaseline(s.Enrolled.Baseline, seen.Baseline); len(changed) > 0 {
		return nil, &Candidate{Reason: CodeBaselineChanged, Enrollment: seen, Changed: changed},
			newError(CodeBaselineChanged, "baseline PCRs %v differ from enrollment", changed)
	}
	return nil, nil, nil
}

// AddPending records a newly issued runtime secret. Its first use commits
// enroll, if the node is not enrolled.
func (s *State) AddPending(secret []byte, ev *Evidence, v *Verified, sent []SentFile, enroll *Enrollment, now time.Time) {
	s.prunePending(now)
	s.Pending = appendBounded(s.Pending, Secret{
		Hash:          HashSecret(secret),
		AKFingerprint: v.AKFingerprint,
		IssuedAt:      now,
		Evidence:      ev,
		Sent:          sent,
		Enroll:        enroll,
	})
}

// AddConfirmation records a secret issued for a changed TPM or baseline. It
// only confirms candidate (see ConfirmCandidate); it never unlocks the runtime
// overlay.
func (s *State) AddConfirmation(secret []byte, candidate *Candidate, now time.Time) {
	s.prunePending(now)
	s.Confirmations = appendBounded(s.Confirmations, Secret{
		Hash:      HashSecret(secret),
		IssuedAt:  now,
		Candidate: candidate,
	})
}

// appendBounded appends e, dropping the oldest entry beyond MaxPending.
func appendBounded(list []Secret, e Secret) []Secret {
	if len(list) >= MaxPending {
		list = list[1:]
	}
	return append(list, e)
}

// IsCurrent reports whether secret is the node's current secret.
func (s *State) IsCurrent(secret []byte) bool {
	return len(secret) > 0 && s.Current != nil &&
		subtle.ConstantTimeCompare([]byte(HashSecret(secret)), []byte(s.Current.Hash)) == 1
}

// CheckSecret reports whether secret is the node's current secret or a
// pending one. A pending secret becomes current, commits the enrollment it was
// issued with, and is recorded as a successful attestation.
func (s *State) CheckSecret(secret []byte, now time.Time) bool {
	if len(secret) == 0 {
		return false
	}
	hash := []byte(HashSecret(secret))
	if s.Current != nil && subtle.ConstantTimeCompare(hash, []byte(s.Current.Hash)) == 1 {
		return true
	}
	s.prunePending(now)
	for _, p := range s.Pending {
		if subtle.ConstantTimeCompare(hash, []byte(p.Hash)) != 1 {
			continue
		}
		if e := p.Enroll; e != nil {
			if s.Enrolled == nil {
				s.Enrolled = e
			} else if s.Enrolled.EKFingerprint != e.EKFingerprint || len(DiffBaseline(s.Enrolled.Baseline, e.Baseline)) > 0 {
				// Another attestation was enrolled first.
				return false
			}
		}
		// The node proved it is in the enrolled state, so any earlier
		// candidate no longer describes it.
		s.Candidate = nil
		p.UsedAt = &now
		p.Enroll = nil
		s.Current = &p
		s.Pending = nil
		s.Record(nil, now)
		return true
	}
	return false
}

// ConfirmCandidate records the candidate issued with secret, and the change
// as the last attempt.
func (s *State) ConfirmCandidate(secret []byte, now time.Time) (*Candidate, bool) {
	if len(secret) == 0 {
		return nil, false
	}
	hash := []byte(HashSecret(secret))
	s.prunePending(now)
	for i, p := range s.Confirmations {
		if subtle.ConstantTimeCompare(hash, []byte(p.Hash)) != 1 {
			continue
		}
		s.Candidate = p.Candidate
		s.Confirmations = append(s.Confirmations[:i:i], s.Confirmations[i+1:]...)
		s.Record(&Error{Code: s.Candidate.Reason, Err: errors.New("change confirmed by the node; awaiting approval")}, now)
		return s.Candidate, true
	}
	return nil, false
}

// HasPendingCandidate reports whether a change awaits confirmation.
func (s *State) HasPendingCandidate() bool {
	return len(s.Confirmations) > 0
}

func (s *State) prunePending(now time.Time) {
	s.Pending = unexpired(s.Pending, now)
	s.Confirmations = unexpired(s.Confirmations, now)
}

func unexpired(list []Secret, now time.Time) []Secret {
	kept := list[:0]
	for _, p := range list {
		if now.Sub(p.IssuedAt) < PendingTTL {
			kept = append(kept, p)
		}
	}
	return kept
}

// Approve accepts the Candidate as the enrollment.
func (s *State) Approve(now time.Time) error {
	if s.Candidate == nil {
		return ErrNoCandidate
	}
	e := s.Candidate.Enrollment
	e.At = now
	s.Enrolled = &e
	s.Candidate = nil
	return nil
}

// Reset forgets the node's secrets so it attests again. With enrollment, it
// also forgets the enrolled TPM, so the next attestation enrolls anew.
func (s *State) Reset(enrollment bool) {
	s.Current = nil
	s.Pending = nil
	s.Confirmations = nil
	if enrollment {
		s.Enrolled = nil
		s.Candidate = nil
	}
}

// Record notes the outcome of an attestation.
func (s *State) Record(err error, now time.Time) {
	a := &Attempt{At: now}
	if err != nil {
		a.Code = CodeOf(err)
		if a.Code == "" {
			a.Code = CodeBadEvidence
		}
		a.Detail = err.Error()
	}
	s.LastAttempt = a
}

// Status summarizes s for display.
func (s *State) Status() string {
	switch {
	case s.Candidate != nil:
		return "CANDIDATE"
	case s.Enrolled != nil:
		return "ENROLLED"
	default:
		return "UNENROLLED"
	}
}

// SecretStatus summarizes the node's secrets for display.
func (s *State) SecretStatus() string {
	switch {
	case s.Current != nil:
		return "current"
	case len(s.Pending) > 0:
		return fmt.Sprintf("pending(%d)", len(s.Pending))
	default:
		return "none"
	}
}
