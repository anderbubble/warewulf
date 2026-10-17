package approve

import (
	"github.com/spf13/cobra"
	"github.com/warewulf/warewulf/internal/app/wwctl/completions"
	"github.com/warewulf/warewulf/internal/pkg/hostlist"
)

type variables struct {
	all       bool
	candidate string
}

func GetCommand() *cobra.Command {
	vars := variables{}
	baseCmd := &cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "approve [OPTIONS] NODE --candidate ID | --all [NODE NAME]",
		Short:                 "Accept a changed TPM or firmware baseline",
		Long: `When a node attests with a different TPM, or with firmware, option ROM, boot
loader or Secure Boot measurements that differ from its enrollment, the server
refuses it and records the change as a candidate. Check the candidate with
"wwctl node tpm verify NODE", and if the change is expected (a firmware update,
a replaced motherboard), approve it by the candidate ID shown there. The ID
ensures you approve the candidate you checked: if another attestation has since
replaced it, approval fails. The node's next attestation succeeds.

--all approves firmware and boot loader changes (baseline_changed) on all
nodes, or on the nodes given. It never approves a different TPM
(ek_changed); approve those individually.
` + hostlist.Docstring,
		RunE:              func(cmd *cobra.Command, args []string) error { return CobraRunE(cmd, args, vars) },
		ValidArgsFunction: completions.Nodes,
		Args:              cobra.ArbitraryArgs,
	}
	baseCmd.Flags().BoolVarP(&vars.all, "all", "a", false, "Approve the baseline changes of all (or the given) nodes")
	baseCmd.Flags().StringVarP(&vars.candidate, "candidate", "c", "", "ID of the candidate to approve, from \"wwctl node tpm verify\"")
	return baseCmd
}
