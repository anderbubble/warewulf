package reset

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/warewulf/warewulf/internal/app/wwctl/node/tpm/report"
	"github.com/warewulf/warewulf/internal/pkg/tpm"
)

func CobraRunE(cmd *cobra.Command, args []string, enrollment bool) error {
	ids, err := report.Nodes(args)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return fmt.Errorf("no nodes with TPM attestation match %v", args)
	}
	store := tpm.DefaultStore()
	for _, id := range ids {
		if !store.Exists(id) {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: no TPM state to reset\n", id)
			continue
		}
		if err := store.Update(id, func(st *tpm.State) error {
			st.Reset(enrollment)
			return nil
		}); err != nil {
			return err
		}
		if enrollment {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: TPM enrollment reset; its next attestation enrolls it\n", id)
		} else {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: TPM secret reset; it will attest again\n", id)
		}
	}
	return nil
}
