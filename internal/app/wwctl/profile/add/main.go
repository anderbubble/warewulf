package add

import (
	"fmt"

	apiprofile "github.com/warewulf/warewulf/internal/pkg/api/profile"
	"github.com/warewulf/warewulf/internal/pkg/warewulfd"

	"github.com/spf13/cobra"
)

func CobraRunE(vars *variables) func(cmd *cobra.Command, args []string) (err error) {
	return func(cmd *cobra.Command, args []string) (err error) {
		add, err := apiprofile.ConvertAddProfile(&apiprofile.AddProfileParameter{
			ProfileConf: vars.profileConf,
			NodeAdd:     vars.nodeAdd,
			NodeNames:   args,
		})
		if err != nil {
			return fmt.Errorf("failed to convert cli inputs to profiles add operation, err: %w", err)
		}
		err = apiprofile.ProfileAdd(add)
		if err != nil {
			return err
		}

		return warewulfd.DaemonReload()
	}
}
