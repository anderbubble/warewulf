package add

import (
	"github.com/spf13/cobra"
	apinode "github.com/warewulf/warewulf/internal/pkg/api/node"
	"github.com/warewulf/warewulf/internal/pkg/warewulfd/daemon"
)

/*
RunE needs a function of type func(*cobraCommand,[]string) err, but
in order to avoid global variables which mess up testing a function of
the required type is returned
*/
func CobraRunE(vars *variables) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		nodeAdd, err := apinode.ConvertAddNode(&apinode.AddNodeParameter{
			NodeConf:  vars.nodeConf,
			NodeAdd:   vars.nodeAdd,
			NodeNames: args,
		})
		if err != nil {
			return err
		}
		err = apinode.NodeAdd(nodeAdd)
		if err != nil {
			return err
		}

		return daemon.DaemonReload()
	}
}
