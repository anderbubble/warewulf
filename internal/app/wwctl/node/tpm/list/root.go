package list

import (
	"github.com/spf13/cobra"
	"github.com/warewulf/warewulf/internal/app/wwctl/completions"
	"github.com/warewulf/warewulf/internal/pkg/hostlist"
)

func GetCommand() *cobra.Command {
	var jsonFlag bool
	baseCmd := &cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "list [OPTIONS] [NODE NAME]",
		Short:                 "List TPM status of nodes",
		Long: `List the TPM status of nodes that use TPM attestation: whether each is
enrolled, has a change awaiting approval (candidate), the result of its last
attestation, and whether it holds a runtime secret.
` + hostlist.Docstring,
		RunE:              func(cmd *cobra.Command, args []string) error { return CobraRunE(cmd, args, jsonFlag) },
		ValidArgsFunction: completions.Nodes,
		Args:              cobra.ArbitraryArgs,
	}
	baseCmd.Flags().BoolVarP(&jsonFlag, "json", "j", false, "Output data in JSON format")
	return baseCmd
}
