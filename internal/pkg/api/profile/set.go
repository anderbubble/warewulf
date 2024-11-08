package apiprofile

import (
	"fmt"

	"dario.cat/mergo"

	"github.com/warewulf/warewulf/internal/pkg/api/routes/wwapiv1"
	"github.com/warewulf/warewulf/internal/pkg/node"
	"github.com/warewulf/warewulf/internal/pkg/util"
	"github.com/warewulf/warewulf/internal/pkg/warewulfd"
	"github.com/warewulf/warewulf/internal/pkg/wwlog"
	"gopkg.in/yaml.v3"
)

// NodeSet is the wwapiv1 implmentation for updating nodeinfo fields.
func ProfileSet(set *wwapiv1.ConfSetParameter) (err error) {
	if set == nil {
		return fmt.Errorf("ProfileAddParameter is nil")
	}
	nodeDB, _, err := ProfileSetParameterCheck(set)
	if err != nil {
		return fmt.Errorf("profile set parameters are wrong: %w", err)
	}
	if err = nodeDB.Persist(); err != nil {
		return err
	}
	if err = warewulfd.DaemonReload(); err != nil {
		return err
	}
	return
}

/*
NodeSetParameterCheck does error checking and returns a modified
NodeYml which than can be persisted
*/
func ProfileSetParameterCheck(set *wwapiv1.ConfSetParameter) (nodeDB node.NodeYaml, count uint, err error) {
	nodeDB, err = node.New()
	if err != nil {
		wwlog.Error("Could not open configuration: %s", err)
		return
	}
	if set == nil {
		err = fmt.Errorf("profile set parameter is nil")
		return
	}
	if set.ConfList == nil {
		err = fmt.Errorf("node nodes to set")
		return
	}
	confs := nodeDB.ListAllProfiles()
	// Note: This does not do expansion on the nodes.
	if set.AllConfs || (len(set.ConfList) == 0) {
		wwlog.Warn("this command will modify all nodes/profiles")
	} else if len(confs) == 0 {
		wwlog.Warn("no nodes/profiles found")
		return
	}
	for _, profileId := range set.ConfList {
		if util.InSlice(set.ConfList, profileId) {
			wwlog.Verbose("evaluating profile: %s", profileId)
			var profilePtr *node.ProfileConf
			profilePtr, err = nodeDB.GetProfilePtr(profileId)
			if err != nil {
				wwlog.Warn("invalid profile: %s", profileId)
				continue
			}
			newProfile := node.EmptyProfile()
			err = yaml.Unmarshal([]byte(set.NodeConfYaml), &newProfile)
			if err != nil {
				return
			}
			// merge in
			err = mergo.Merge(profilePtr, &newProfile, mergo.WithOverride)
			if err != nil {
				return
			}

			if set.NetdevDelete != "" {
				if _, ok := profilePtr.NetDevs[set.NetdevDelete]; !ok {
					err = fmt.Errorf("network device name doesn't exist: %s", set.NetdevDelete)
					wwlog.Error(fmt.Sprintf("%v", err.Error()))
					return
				}
				wwlog.Verbose("Profile: %s, Deleting network device: %s", profileId, set.NetdevDelete)
				delete(profilePtr.NetDevs, set.NetdevDelete)
			}
			if set.PartitionDelete != "" {
				for diskname, disk := range profilePtr.Disks {
					if _, ok := disk.Partitions[set.PartitionDelete]; ok {
						wwlog.Verbose("Node: %s, on disk %, deleting partition: %s", profileId, diskname, set.PartitionDelete)
						delete(disk.Partitions, set.PartitionDelete)
					} else {
						return nodeDB, count, fmt.Errorf("partition doesn't exist: %s", set.PartitionDelete)

					}
				}
			}
			if set.DiskDelete != "" {
				if _, ok := profilePtr.Disks[set.DiskDelete]; ok {
					wwlog.Verbose("Node: %s, deleting disk: %s", profileId, set.DiskDelete)
					delete(profilePtr.Disks, set.DiskDelete)
				} else {
					return nodeDB, count, fmt.Errorf("disk doesn't exist: %s", set.DiskDelete)
				}
			}
			if set.FilesystemDelete != "" {
				if _, ok := profilePtr.FileSystems[set.FilesystemDelete]; ok {
					wwlog.Verbose("Node: %s, deleting filesystem: %s", profileId, set.FilesystemDelete)
					delete(profilePtr.FileSystems, set.FilesystemDelete)
				} else {
					return nodeDB, count, fmt.Errorf("disk doesn't exist: %s", set.FilesystemDelete)
				}
			}
			for _, key := range set.TagDel {
				delete(profilePtr.Tags, key)
			}
			for key, val := range set.TagAdd {
				if profilePtr.Tags == nil {
					profilePtr.Tags = make(map[string]string)
				}
				profilePtr.Tags[key] = val
			}
			for key, val := range set.IpmiTagAdd {
				if profilePtr.Ipmi.Tags == nil {
					profilePtr.Ipmi.Tags = make(map[string]string)
				}
				profilePtr.Ipmi.Tags[key] = val
			}
			for _, key := range set.IpmiTagDel {
				delete(profilePtr.Ipmi.Tags, key)
			}
			if _, ok := profilePtr.NetDevs[set.Netdev]; ok {
				for _, key := range set.NetTagDel {
					delete(profilePtr.NetDevs[set.Netdev].Tags, key)
				}
				for key, val := range set.TagAdd {
					profilePtr.NetDevs[set.Netdev].Tags[key] = val
				}
			}
			count++
		}
	}
	return
}
