package tpm

import (
	"github.com/spf13/cobra"
	"github.com/warewulf/warewulf/internal/app/wwctl/node/tpm/approve"
	"github.com/warewulf/warewulf/internal/app/wwctl/node/tpm/list"
	"github.com/warewulf/warewulf/internal/app/wwctl/node/tpm/reset"
	"github.com/warewulf/warewulf/internal/app/wwctl/node/tpm/verify"
)

func GetCommand() *cobra.Command {
	baseCmd := &cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "tpm COMMAND [OPTIONS]",
		Short:                 "Manage TPM attestation of nodes",
		Long:                  "Manage TPM enrollment and attestation of nodes that use TPM (--tpm=true).",
	}
	baseCmd.AddCommand(list.GetCommand())
	baseCmd.AddCommand(verify.GetCommand())
	baseCmd.AddCommand(reset.GetCommand())
	baseCmd.AddCommand(approve.GetCommand())
	return baseCmd
}
