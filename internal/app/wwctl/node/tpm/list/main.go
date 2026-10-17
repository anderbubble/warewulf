package list

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/warewulf/warewulf/internal/app/wwctl/node/tpm/report"
	"github.com/warewulf/warewulf/internal/app/wwctl/table"
	"github.com/warewulf/warewulf/internal/pkg/tpm"
)

// NodeTpmStatus is one node in `wwctl node tpm list --json`.
type NodeTpmStatus struct {
	Node          string       `json:"node"`
	State         string       `json:"state"`
	Manufacturer  string       `json:"manufacturer,omitempty"`
	EKFingerprint string       `json:"ek_fingerprint,omitempty"`
	Candidate     *Candidate   `json:"candidate,omitempty"`
	LastAttempt   *tpm.Attempt `json:"last_attempt,omitempty"`
	Secret        string       `json:"secret"`
}

// Candidate is a change awaiting approval.
type Candidate struct {
	ID            string   `json:"id"`
	Reason        tpm.Code `json:"reason"`
	EKFingerprint string   `json:"ek_fingerprint"`
	Changed       []int    `json:"changed_pcrs,omitempty"`
}

func CobraRunE(cmd *cobra.Command, args []string, jsonFlag bool) error {
	ids, err := report.Nodes(args)
	if err != nil {
		return err
	}
	store := tpm.DefaultStore()
	var statuses []NodeTpmStatus
	for _, id := range ids {
		st, err := store.Load(id)
		if err != nil {
			return err
		}
		s := NodeTpmStatus{Node: id, State: st.Status(), LastAttempt: st.LastAttempt, Secret: st.SecretStatus()}
		if e := st.Enrolled; e != nil {
			s.Manufacturer, s.EKFingerprint = tpm.Manufacturer(e.EKCert), e.EKFingerprint
		}
		if c := st.Candidate; c != nil {
			s.Candidate = &Candidate{ID: c.ID(), Reason: c.Reason, EKFingerprint: c.EKFingerprint, Changed: c.Changed}
		}
		statuses = append(statuses, s)
	}

	if jsonFlag {
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(statuses); err != nil {
			return fmt.Errorf("failed to output JSON: %w", err)
		}
		return nil
	}

	t := table.New(cmd.OutOrStdout())
	t.AddHeader("NODE", "STATE", "MANUFACTURER", "EK (SHA256)", "LAST ATTEST", "SECRET")
	for _, s := range statuses {
		ek := s.EKFingerprint
		if len(ek) > 16 {
			ek = ek[:16] + "..."
		}
		t.AddLine(table.Prep([]string{s.Node, s.State, s.Manufacturer, ek, report.LastAttempt(s.LastAttempt), s.Secret})...)
	}
	t.Print()
	return nil
}
