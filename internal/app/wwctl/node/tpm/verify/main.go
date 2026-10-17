package verify

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/warewulf/warewulf/internal/app/wwctl/node/tpm/report"
	"github.com/warewulf/warewulf/internal/pkg/tpm"
)

func CobraRunE(vars *variables) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if len(vars.files) > 0 {
			return verifyFiles(cmd, vars)
		}
		ids, err := report.Nodes(args)
		if err != nil {
			return err
		}
		store := tpm.DefaultStore()
		failed := 0
		for _, id := range ids {
			st, err := store.Load(id)
			if err != nil {
				return err
			}
			if err := report.Write(cmd.OutOrStdout(), id, st, vars.pcrFilter, vars.displayEvent); err != nil {
				failed++
			}
		}
		if failed > 0 {
			return fmt.Errorf("TPM evidence failed to verify for %d node(s)", failed)
		}
		return nil
	}
}

// verifyFiles reports on TPM state files given with --file.
func verifyFiles(cmd *cobra.Command, vars *variables) error {
	failed := 0
	for _, name := range vars.files {
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		var st tpm.State
		if err := json.Unmarshal(data, &st); err != nil {
			return fmt.Errorf("reading %s: %w", name, err)
		}
		if st.Version != tpm.StateVersion {
			return fmt.Errorf("reading %s: not a TPM state file (version %d)", name, st.Version)
		}
		if err := report.Write(cmd.OutOrStdout(), name, &st, vars.pcrFilter, vars.displayEvent); err != nil {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("TPM evidence failed to verify in %d file(s)", failed)
	}
	return nil
}
