// Package report holds what the wwctl node tpm commands share: choosing
// nodes and describing a node's TPM state.
package report

import (
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/warewulf/warewulf/internal/pkg/hostlist"
	"github.com/warewulf/warewulf/internal/pkg/node"
	"github.com/warewulf/warewulf/internal/pkg/tpm"
)

// Nodes returns the IDs of the nodes named by args (hostlist patterns), or
// of all nodes if args is empty, that use TPM or have TPM state.
func Nodes(args []string) ([]string, error) {
	nodeDB, err := node.New()
	if err != nil {
		return nil, err
	}
	nodes, err := nodeDB.FindAllNodes()
	if err != nil {
		return nil, err
	}
	names := hostlist.Expand(args)
	if len(names) > 0 {
		nodes = node.FilterNodeListByName(nodes, names)
	}
	store := tpm.DefaultStore()
	var ids []string
	for _, n := range nodes {
		if n.Tpm() || store.Exists(n.Id()) {
			ids = append(ids, n.Id())
		}
	}
	slices.Sort(ids)
	return ids, nil
}

// Time formats t for display.
func Time(t time.Time) string {
	if t.IsZero() {
		return "--"
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

// LastAttempt describes the outcome of the last attestation.
func LastAttempt(a *tpm.Attempt) string {
	switch {
	case a == nil:
		return "--"
	case a.OK():
		return "ok " + Time(a.At)
	default:
		return string(a.Code) + " " + Time(a.At)
	}
}

// Write describes node's TPM state to w. It re-verifies the evidence behind
// the node's current (or newest pending) secret, and returns an error if that
// fails. With showLog, it includes the evidence's event log, limited to
// pcrFilter if it is not empty.
func Write(w io.Writer, name string, st *tpm.State, pcrFilter []int, showLog bool) error {
	_, _ = fmt.Fprintf(w, "%s: %s\n", name, st.Status())
	if e := st.Enrolled; e != nil {
		_, _ = fmt.Fprintf(w, "  enrolled TPM:  %s, EK sha256 %s (since %s)\n", tpm.Manufacturer(e.EKCert), e.EKFingerprint, Time(e.At))
	}
	if c := st.Candidate; c != nil {
		_, _ = fmt.Fprintf(w, "  candidate:     %s: %s", c.ID(), c.Reason)
		if len(c.Changed) > 0 {
			_, _ = fmt.Fprintf(w, ", PCRs %v differ", c.Changed)
		}
		_, _ = fmt.Fprintf(w, "; %s, EK sha256 %s (seen %s)\n", tpm.Manufacturer(c.EKCert), c.EKFingerprint, Time(c.At))
		_, _ = fmt.Fprintf(w, "                 if expected, accept it with: wwctl node tpm approve %s --candidate %s\n", name, c.ID())
	}
	_, _ = fmt.Fprintf(w, "  last attest:   %s\n", LastAttempt(st.LastAttempt))
	if a := st.LastAttempt; a != nil && !a.OK() && a.Detail != "" {
		_, _ = fmt.Fprintf(w, "                 %s\n", a.Detail)
	}
	_, _ = fmt.Fprintf(w, "  secret:        %s\n", st.SecretStatus())

	// Evidence behind a pending secret is unproven until the node uses the
	// secret (see tpm.State.Evaluate).
	sec, unconfirmed := st.Current, ""
	if sec == nil && len(st.Pending) > 0 {
		sec = &st.Pending[len(st.Pending)-1]
		unconfirmed = ", unconfirmed: the node hasn't used this secret yet"
	}
	if sec == nil || sec.Evidence == nil {
		return nil
	}
	ev := sec.Evidence
	v, err := tpm.VerifyEvidence(ev, ev.Nonce, sec.Sent)
	if err == nil && st.Enrolled != nil && v.EKFingerprint != st.Enrolled.EKFingerprint {
		err = fmt.Errorf("evidence is from EK %s, not the enrolled TPM", v.EKFingerprint)
	}
	if err != nil {
		_, _ = fmt.Fprintf(w, "  evidence:      FAILED: %s\n", err)
	} else {
		_, _ = fmt.Fprintf(w, "  evidence:      verified%s (issued %s, AK sha256 %s)\n", unconfirmed, Time(sec.IssuedAt), sec.AKFingerprint)
		for _, f := range sec.Sent {
			_, _ = fmt.Fprintf(w, "    sent %-9s %s %s\n", f.Stage, f.Digest, f.Path)
		}
	}
	if showLog {
		log, logErr := tpm.FormatEventLog(ev.EventLog, pcrFilter)
		if logErr != nil {
			_, _ = fmt.Fprintf(w, "  event log:     %s\n", logErr)
		} else {
			_, _ = fmt.Fprint(w, log)
		}
	}
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}
