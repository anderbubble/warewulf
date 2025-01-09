package apinode

import (
	"encoding/hex"
	"fmt"
	"net"
	"strings"

	"github.com/warewulf/warewulf/internal/pkg/api/routes/wwapiv1"
	"github.com/warewulf/warewulf/internal/pkg/hostlist"
	"github.com/warewulf/warewulf/internal/pkg/node"
	"github.com/warewulf/warewulf/internal/pkg/util"
	"github.com/warewulf/warewulf/internal/pkg/wwlog"
	"gopkg.in/yaml.v3"
)

type AddNodeParameter struct {
	NodeConf  node.Node
	NodeAdd   node.NodeConfAdd
	NodeNames []string
}

func ConvertAddNode(addNode *AddNodeParameter) (*wwapiv1.NodeAddParameter, error) {
	// remove the UNDEF network as all network values are assigned
	// to this network
	if !node.ObjectIsEmpty(addNode.NodeConf.NetDevs["UNDEF"]) {
		netDev := *addNode.NodeConf.NetDevs["UNDEF"]
		addNode.NodeConf.NetDevs[addNode.NodeAdd.Net] = &netDev
	}
	delete(addNode.NodeConf.NetDevs, "UNDEF")
	if addNode.NodeAdd.FsName != "" {
		if !strings.HasPrefix(addNode.NodeAdd.FsName, "/dev") {
			if addNode.NodeAdd.FsName == addNode.NodeAdd.PartName {
				addNode.NodeAdd.FsName = "/dev/disk/by-partlabel/" + addNode.NodeAdd.PartName
			} else {
				return nil, fmt.Errorf("filesystems need to have a underlying blockdev")
			}
		}
		fs := *addNode.NodeConf.FileSystems["UNDEF"]
		addNode.NodeConf.FileSystems[addNode.NodeAdd.FsName] = &fs
	}
	delete(addNode.NodeConf.FileSystems, "UNDEF")
	if addNode.NodeAdd.DiskName != "" && addNode.NodeAdd.PartName != "" {
		prt := *addNode.NodeConf.Disks["UNDEF"].Partitions["UNDEF"]
		addNode.NodeConf.Disks["UNDEF"].Partitions[addNode.NodeAdd.PartName] = &prt
		delete(addNode.NodeConf.Disks["UNDEF"].Partitions, "UNDEF")
		dsk := *addNode.NodeConf.Disks["UNDEF"]
		addNode.NodeConf.Disks[addNode.NodeAdd.DiskName] = &dsk
	}
	if (addNode.NodeAdd.DiskName != "") != (addNode.NodeAdd.PartName != "") {
		return nil, fmt.Errorf("partition and disk must be specified")
	}
	delete(addNode.NodeConf.Disks, "UNDEF")
	if len(addNode.NodeConf.Profiles) == 0 {
		addNode.NodeConf.Profiles = []string{"default"}
	}
	buffer, err := yaml.Marshal(addNode.NodeConf)
	if err != nil {
		wwlog.Error("Can't marshall nodeInfo", err)
		return nil, err
	}
	return &wwapiv1.NodeAddParameter{
		NodeConfYaml: string(buffer[:]),
		NodeNames:    addNode.NodeNames,
		Force:        true,
	}, nil
}

// NodeAdd adds nodes for management by Warewulf.
func NodeAdd(nap *wwapiv1.NodeAddParameter) (err error) {
	if nap == nil {
		return fmt.Errorf("NodeAddParameter is nil")
	}

	nodeDB, err := node.New()
	if err != nil {
		return fmt.Errorf("failed to open node database: %w", err)
	}
	dbHash := nodeDB.Hash()
	if hex.EncodeToString(dbHash[:]) != nap.Hash && !nap.Force {
		return fmt.Errorf("got wrong hash, not modifying node database")
	}
	node_args := hostlist.Expand(nap.NodeNames)
	var ipv4, ipmiaddr net.IP
	for _, a := range node_args {
		n, err := nodeDB.AddNode(a)
		if err != nil {
			return fmt.Errorf("failed to add node: %w", err)
		}
		err = yaml.Unmarshal([]byte(nap.NodeConfYaml), &n)
		if err != nil {
			return fmt.Errorf("Failed to decode NodeConf: %w", err)
		}
		wwlog.Info("Added node: %s", a)
		for _, dev := range n.NetDevs {
			if !ipv4.IsUnspecified() && ipv4 != nil {
				// if more nodes are added increment IPv4 address
				ipv4 = util.IncrementIPv4(ipv4, 1)
				wwlog.Verbose("Incremented IP addr to %s", ipv4)
				dev.Ipaddr = ipv4

			} else if !dev.Ipaddr.IsUnspecified() {
				ipv4 = dev.Ipaddr
			}
		}
		if n.Ipmi != nil {
			if !ipmiaddr.IsUnspecified() && ipmiaddr != nil {
				ipmiaddr = util.IncrementIPv4(ipmiaddr, 1)
				wwlog.Verbose("Incremented ipmi IP addr to %s", ipmiaddr)
				n.Ipmi.Ipaddr = ipmiaddr
			} else if !n.Ipmi.Ipaddr.IsUnspecified() {
				ipmiaddr = n.Ipmi.Ipaddr
			}
		}
	}

	err = nodeDB.Persist()
	if err != nil {
		return fmt.Errorf("failed to persist new node: %w", err)
	}
	return
}
