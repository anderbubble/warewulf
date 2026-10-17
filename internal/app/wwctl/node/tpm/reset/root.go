package reset

import (
	"github.com/spf13/cobra"
	"github.com/warewulf/warewulf/internal/app/wwctl/completions"
	"github.com/warewulf/warewulf/internal/pkg/hostlist"
)

func GetCommand() *cobra.Command {
	var enrollment bool
	baseCmd := &cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "reset [OPTIONS] NODE NAME",
		Short:                 "Make nodes attest again",
		Long: `Discard the runtime secrets of nodes, so each attests again the next time it
requests its runtime overlay. With --enrollment, also forget the enrolled TPM
and baseline: the node's next attestation enrolls whatever TPM it presents.
Use --enrollment when a node's hardware is replaced; to accept a firmware or
hardware change the server has already seen, use "wwctl node tpm approve".
` + hostlist.Docstring,
		RunE:              func(cmd *cobra.Command, args []string) error { return CobraRunE(cmd, args, enrollment) },
		ValidArgsFunction: completions.Nodes,
		Args:              cobra.MinimumNArgs(1),
	}
	baseCmd.Flags().BoolVar(&enrollment, "enrollment", false, "Also forget the enrolled TPM and baseline")
	return baseCmd
}
