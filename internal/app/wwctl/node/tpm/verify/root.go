package verify

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/warewulf/warewulf/internal/app/wwctl/completions"
	"github.com/warewulf/warewulf/internal/pkg/hostlist"
)

type variables struct {
	pcrFilter    []int
	displayEvent bool
	files        []string
}

func GetCommand() *cobra.Command {
	vars := variables{}
	baseCmd := &cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "verify [OPTIONS] [NODE NAME] | --file FILE",
		Short:                 "Verify and describe the TPM state of nodes",
		Long: `Describe each node's TPM enrollment, any change awaiting approval, and its last
attestation, and re-verify the evidence behind its runtime secret: the quote,
the event log, and the boot files the server sent. Exits non-zero if any
node's evidence fails to verify.

With --file, do the same for TPM state files instead of configured nodes: copies
of a node's state from the TPM state directory (LOCALSTATEDIR/warewulf/tpm/
NODE.json), for example from a backup or another server.
` + hostlist.Docstring,
		RunE:              CobraRunE(&vars),
		ValidArgsFunction: completions.Nodes,
		Args:              cobra.ArbitraryArgs,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("pcr") && !vars.displayEvent {
				return fmt.Errorf("flag --pcr requires --eventlog")
			}
			if len(vars.files) > 0 && len(args) > 0 {
				return fmt.Errorf("give either nodes or --file, not both")
			}
			return nil
		},
	}
	baseCmd.Flags().IntSliceVar(&vars.pcrFilter, "pcr", []int{}, "Show only these PCRs of the event log (comma separated)")
	baseCmd.Flags().BoolVar(&vars.displayEvent, "eventlog", false, "Show the event log")
	baseCmd.Flags().StringSliceVar(&vars.files, "file", nil, "Verify TPM state files instead of configured nodes (repeatable)")
	return baseCmd
}
