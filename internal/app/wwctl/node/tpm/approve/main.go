package approve

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/warewulf/warewulf/internal/app/wwctl/node/tpm/report"
	"github.com/warewulf/warewulf/internal/pkg/tpm"
)

func CobraRunE(cmd *cobra.Command, args []string, vars variables) error {
	switch {
	case vars.all && vars.candidate != "":
		return errors.New("--candidate approves one node; it can't be combined with --all")
	case !vars.all && len(args) == 0:
		return errors.New("no node specified; give a node and --candidate, or use --all")
	}
	ids, err := report.Nodes(args)
	if err != nil {
		return err
	}
	if vars.all {
		return approveAll(cmd, ids)
	}
	if len(ids) != 1 {
		if vars.candidate == "" {
			return fmt.Errorf("give exactly one node with --candidate ID, or use --all; %v matches %d nodes", args, len(ids))
		}
		return fmt.Errorf("--candidate approves exactly one node; %v matches %d", args, len(ids))
	}
	return approveOne(cmd, ids[0], vars.candidate)
}

// hasCandidate reports whether node has a candidate, without writing state.
func hasCandidate(id string) (bool, error) {
	st, err := tpm.DefaultStore().Load(id)
	if err != nil {
		return false, err
	}
	return st.Candidate != nil, nil
}

func approveOne(cmd *cobra.Command, id, candidateID string) error {
	if ok, err := hasCandidate(id); err != nil {
		return err
	} else if !ok {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: nothing to approve\n", id)
		return nil
	}
	var c tpm.Candidate
	err := tpm.DefaultStore().Update(id, func(st *tpm.State) error {
		if st.Candidate == nil {
			return tpm.ErrNoCandidate
		}
		c = *st.Candidate
		if candidateID == "" || c.ID() != candidateID {
			return errCandidateMismatch
		}
		return st.Approve(time.Now())
	})
	switch {
	case errors.Is(err, tpm.ErrNoCandidate):
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: nothing to approve\n", id)
		return nil
	case errors.Is(err, errCandidateMismatch):
		if candidateID == "" {
			return fmt.Errorf("%s has candidate %s (%s); check it with \"wwctl node tpm verify %s\", then approve it with --candidate %s",
				id, c.ID(), c.Reason, id, c.ID())
		}
		return fmt.Errorf("%s's candidate is now %s (%s), not %s; check it again with \"wwctl node tpm verify %s\"",
			id, c.ID(), c.Reason, candidateID, id)
	case err != nil:
		return err
	}
	printApproved(cmd, id, &c)
	return nil
}

func approveAll(cmd *cobra.Command, ids []string) error {
	for _, id := range ids {
		if ok, err := hasCandidate(id); err != nil {
			return err
		} else if !ok {
			continue
		}
		var c tpm.Candidate
		skipped := false
		err := tpm.DefaultStore().Update(id, func(st *tpm.State) error {
			if st.Candidate == nil {
				return tpm.ErrNoCandidate
			}
			c = *st.Candidate
			if c.Reason != tpm.CodeBaselineChanged {
				skipped = true
				return nil
			}
			return st.Approve(time.Now())
		})
		switch {
		case errors.Is(err, tpm.ErrNoCandidate):
		case err != nil:
			return err
		case skipped:
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: skipped %s candidate %s; approve it individually with --candidate\n", id, c.Reason, c.ID())
		default:
			printApproved(cmd, id, &c)
		}
	}
	return nil
}

var errCandidateMismatch = errors.New("candidate mismatch")

func printApproved(cmd *cobra.Command, id string, c *tpm.Candidate) {
	detail := ""
	if len(c.Changed) > 0 {
		detail = fmt.Sprintf(", PCRs %v", c.Changed)
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: approved candidate %s: %s%s; EK sha256 %s\n", id, c.ID(), c.Reason, detail, c.EKFingerprint)
}
