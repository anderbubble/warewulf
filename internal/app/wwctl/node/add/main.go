package add

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/spf13/cobra"
	apinode "github.com/warewulf/warewulf/internal/pkg/api/node"
	"github.com/warewulf/warewulf/internal/pkg/api/routes/wwapiv1"
	"github.com/warewulf/warewulf/internal/pkg/node"
	"github.com/warewulf/warewulf/internal/pkg/wwlog"
)

/*
RunE needs a function of type func(*cobraCommand,[]string) err, but
in order to avoid global variables which mess up testing a function of
the required type is returned
*/
func CobraRunE(vars *variables) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		// remove the UNDEF network as all network values are assigned
		// to this network
		if !node.ObjectIsEmpty(vars.node.NetDevs["UNDEF"]) {
			netDev := *vars.node.NetDevs["UNDEF"]
			vars.node.NetDevs[vars.netName] = &netDev
			fmt.Println("not empty")
		}
		delete(vars.node.NetDevs, "UNDEF")
		if vars.fsName != "" {
			if !strings.HasPrefix(vars.fsName, "/dev") {
				if vars.fsName == vars.partName {
					vars.fsName = "/dev/disk/by-partlabel/" + vars.partName
				} else {
					return fmt.Errorf("filesystems need to have a underlying blockdev")
				}
			}
			fs := *vars.node.FileSystems["UNDEF"]
			vars.node.FileSystems[vars.fsName] = &fs
		}
		delete(vars.node.FileSystems, "UNDEF")
		if vars.diskName != "" && vars.partName != "" {
			prt := *vars.node.Disks["UNDEF"].Partitions["UNDEF"]
			vars.node.Disks["UNDEF"].Partitions[vars.partName] = &prt
			delete(vars.node.Disks["UNDEF"].Partitions, "UNDEF")
			dsk := *vars.node.Disks["UNDEF"]
			vars.node.Disks[vars.diskName] = &dsk
		}
		if (vars.diskName != "") != (vars.partName != "") {
			return fmt.Errorf("partition and disk must be specified")
		}
		delete(vars.node.Disks, "UNDEF")
		if len(vars.node.Profiles) == 0 {
			vars.node.Profiles = []string{"default"}
		}
		buffer, err := yaml.Marshal(vars.node)
		if err != nil {
			wwlog.Error("Can't marshall nodeInfo", err)
			return err
		}
		set := wwapiv1.NodeAddParameter{
			NodeYaml:  string(buffer[:]),
			NodeNames: args,
			Force:     true,
		}
		return apinode.NodeAdd(&set)
	}
}
