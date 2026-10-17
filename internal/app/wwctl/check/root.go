package check

import (
	"github.com/spf13/cobra"
	"github.com/warewulf/warewulf/internal/app/wwctl/completions"
)

func GetCommand() *cobra.Command {
	return &cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "check",
		Short:                 "Check the cluster configuration for problems",
		Long: `Check the server and node configuration for problems that would keep nodes
from provisioning or working as configured, and report each with the nodes it
affects. It currently checks the server settings TPM attestation needs, and
TPM changes awaiting approval. Exits non-zero if it finds an error.`,
		RunE:              CobraRunE,
		Args:              cobra.NoArgs,
		ValidArgsFunction: completions.None,
	}
}
