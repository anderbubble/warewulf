package apiprofile

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/warewulf/warewulf/internal/pkg/api/routes/wwapiv1"
	"github.com/warewulf/warewulf/internal/pkg/node"
	"github.com/warewulf/warewulf/internal/pkg/util"
	"gopkg.in/yaml.v3"
)

type AddProfileParameter struct {
	ProfileConf node.Profile
	NodeAdd     node.NodeConfAdd
	NodeNames   []string
}

func ConvertAddProfile(addProfile *AddProfileParameter) (*wwapiv1.NodeAddParameter, error) {
	// remove the UNDEF network as all network values are assigned
	// to this network
	if !node.ObjectIsEmpty(addProfile.ProfileConf.NetDevs["UNDEF"]) {
		netDev := *addProfile.ProfileConf.NetDevs["UNDEF"]
		addProfile.ProfileConf.NetDevs[addProfile.NodeAdd.Net] = &netDev
	}
	delete(addProfile.ProfileConf.NetDevs, "UNDEF")
	if addProfile.NodeAdd.FsName != "" {
		if !strings.HasPrefix(addProfile.NodeAdd.FsName, "/dev") {
			if addProfile.NodeAdd.FsName == addProfile.NodeAdd.PartName {
				addProfile.NodeAdd.FsName = "/dev/disk/by-partlabel/" + addProfile.NodeAdd.PartName
			} else {
				return nil, fmt.Errorf("filesystems need to have a underlying blockdev")
			}
		}
		fs := *addProfile.ProfileConf.FileSystems["UNDEF"]
		addProfile.ProfileConf.FileSystems[addProfile.NodeAdd.FsName] = &fs
	}
	delete(addProfile.ProfileConf.FileSystems, "UNDEF")
	if addProfile.NodeAdd.DiskName != "" && addProfile.NodeAdd.PartName != "" {
		prt := *addProfile.ProfileConf.Disks["UNDEF"].Partitions["UNDEF"]
		addProfile.ProfileConf.Disks["UNDEF"].Partitions[addProfile.NodeAdd.PartName] = &prt
		delete(addProfile.ProfileConf.Disks["UNDEF"].Partitions, "UNDEF")
		dsk := *addProfile.ProfileConf.Disks["UNDEF"]
		addProfile.ProfileConf.Disks[addProfile.NodeAdd.DiskName] = &dsk
	}
	if (addProfile.NodeAdd.DiskName != "") != (addProfile.NodeAdd.PartName != "") {
		return nil, fmt.Errorf("partition and disk must be specified")
	}
	delete(addProfile.ProfileConf.Disks, "UNDEF")
	buffer, err := yaml.Marshal(addProfile.ProfileConf)
	if err != nil {
		return nil, fmt.Errorf("can not marshall nodeInfo: %w", err)
	}
	return &wwapiv1.NodeAddParameter{
		NodeConfYaml: string(buffer[:]),
		NodeNames:    addProfile.NodeNames,
		Force:        true,
	}, nil
}

/*
Adds a new profile with the given name
*/
func ProfileAdd(nsp *wwapiv1.NodeAddParameter) error {
	if nsp == nil {
		return fmt.Errorf("NodeAddParameter is nill")
	}
	nodeDB, err := node.New()
	if err != nil {
		return fmt.Errorf("Could not open database: %w", err)
	}
	for _, p := range nsp.NodeNames {
		if util.InSlice(nodeDB.ListAllProfiles(), p) {
			return errors.New(fmt.Sprintf("profile with name %s already exists", p))
		}
		pNew, err := nodeDB.AddProfile(p)
		if err != nil {
			return err
		}
		err = yaml.Unmarshal([]byte(nsp.NodeConfYaml), &pNew)
		if err != nil {
			return fmt.Errorf("failed to add profile: %w", err)
		}
	}
	err = nodeDB.Persist()
	if err != nil {
		return fmt.Errorf("failed to persist new profile: %w", err)
	}
	return nil
}
