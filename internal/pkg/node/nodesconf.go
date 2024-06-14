package node

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"net"
	"os"
	"path"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"dario.cat/mergo"
	warewulfconf "github.com/warewulf/warewulf/internal/pkg/config"
	"github.com/warewulf/warewulf/internal/pkg/wwlog"

	"gopkg.in/yaml.v3"
)

var (
	ConfigFile string
)

/*
Structure of which goes to disk
*/
type NodesConf struct {
	WWInternal   int `yaml:"WW_INTERNAL,omitempty" json:"WW_INTERNAL,omitempty"`
	nodeProfiles map[string]*Profile
	nodes        map[string]*Node
}

/*
Creates a new nodeDb object from the on-disk configuration
*/
func NewNodesConf() (NodesConf, error) {
	conf := warewulfconf.Get()
	if ConfigFile == "" {
		ConfigFile = path.Join(conf.Paths.Sysconfdir, "warewulf/nodes.conf")
	}
	wwlog.Verbose("Opening node configuration file: %s", ConfigFile)
	data, err := os.ReadFile(ConfigFile)
	if err != nil {
		return NodesConf{}, err
	}
	return ParseNodesConf(data)
}

// ParseNodesConf constructs a new nodeDb object from an input YAML
// document. Passes any errors return from yaml.Unmarshal. Returns an
// error if any parsed value is not of a valid type for the given
// parameter.
func ParseNodesConf(data []byte) (nodeList NodesConf, err error) {
	wwlog.Debug("Unmarshaling the node configuration")
	err = yaml.Unmarshal(data, &nodeList)
	if err != nil {
		return nodeList, err
	}
	wwlog.Debug("Checking nodes for types")
	if nodeList.nodes == nil {
		nodeList.nodes = map[string]*Node{}
	}
	if nodeList.nodeProfiles == nil {
		nodeList.nodeProfiles = map[string]*Profile{}
	}
	wwlog.Debug("returning node object")
	return nodeList, nil
}

/*
Get a node with its merged in nodes
*/
func (config *NodesConf) GetNode(id string) (node Node, err error) {
	if _, ok := config.nodes[id]; !ok {
		return node, ErrNotFound
	}
	node = EmptyNode()
	// create a deep copy of the node, as otherwise pointers
	// and not their contents is merged
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	dec := gob.NewDecoder(&buf)
	err = enc.Encode(config.nodes[id])
	if err != nil {
		return node, err
	}
	err = dec.Decode(&node)
	if err != nil {
		return node, err
	}
	for _, p := range cleanList(config.nodes[id].Profiles) {
		includedProfile, err := config.GetProfile(p)
		if err != nil {
			wwlog.Warn("profile not found: %s", p)
			continue
		}
		err = mergo.Merge(&node.Profile, includedProfile, mergo.WithAppendSlice)
		if err != nil {
			return node, err
		}
	}
	// finally set no exported values
	node.id = id
	node.valid = true
	if netdev, ok := node.NetDevs[node.PrimaryNetDev]; ok {
		netdev.primary = true
	} else {
		keys := make([]string, 0)
		for k := range node.NetDevs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if len(keys) > 0 {
			wwlog.Debug("%s: no primary defined, sanitizing to: %s", id, keys[0])
			node.NetDevs[keys[0]].primary = true
			node.PrimaryNetDev = keys[0]
		}
	}
	wwlog.Debug("constructed node: %s", id)
	return
}

/*
Return the node with the id string without the merged in nodes, return ErrNotFound
otherwise
*/
func (config *NodesConf) GetNodeOnly(id string) (node Node, err error) {
	node = EmptyNode()
	if found, ok := config.nodes[id]; ok {
		return *found, nil
	}
	return node, ErrNotFound
}

/*
Return pointer to the  node with the id string without the merged in nodes, return ErrMotFound
otherwise
*/
func (config *NodesConf) GetNodeOnlyPtr(id string) (*Node, error) {
	node := EmptyNode()
	if found, ok := config.nodes[id]; ok {
		return found, nil
	}
	return &node, ErrNotFound
}

/*
Get the profile with id, return ErrNotFound otherwise
*/
func (config *NodesConf) GetProfile(id string) (profile Profile, err error) {
	if found, ok := config.nodeProfiles[id]; ok {
		found.id = id
		return *found, nil
	}
	return profile, ErrNotFound
}

/*
Get the profile with id, return ErrNotFound otherwise
*/
func (config *NodesConf) GetProfilePtr(id string) (profile *Profile, err error) {
	if found, ok := config.nodeProfiles[id]; ok {
		found.id = id
		return found, nil
	}
	return profile, ErrNotFound
}

/*
Get the nodes from the loaded configuration. This function also merges
the nodes with the given nodes.
*/
func (config *NodesConf) FindAllNodes(nodes ...string) (nodeList []Node, err error) {
	if len(nodes) == 0 {
		for n := range config.nodes {
			nodes = append(nodes, n)
		}
	}
	wwlog.Debug("Finding nodes: %s", nodes)
	for _, nodeId := range nodes {
		node, err := config.GetNode(nodeId)
		if err != nil {
			return nodeList, err
		}
		nodeList = append(nodeList, node)
	}
	sort.Slice(nodeList, func(i, j int) bool {
		if nodeList[i].ClusterName < nodeList[j].ClusterName {
			return true
		} else if nodeList[i].ClusterName == nodeList[j].ClusterName {
			if nodeList[i].id < nodeList[j].id {
				return true
			}
		}
		return false
	})
	return nodeList, nil
}

/*
Return all nodes as Profiles
*/
func (config *NodesConf) FindAllProfiles(nodes ...string) (profileList []Profile, err error) {
	if len(nodes) == 0 {
		for n := range config.nodeProfiles {
			nodes = append(nodes, n)
		}
	}
	wwlog.Debug("Finding nodes: %s", nodes)
	for _, profileId := range nodes {
		node, err := config.GetProfile(profileId)
		if err != nil {
			return profileList, err
		}
		profileList = append(profileList, node)
	}
	sort.Slice(profileList, func(i, j int) bool {
		if profileList[i].ClusterName < profileList[j].ClusterName {
			return true
		} else if profileList[i].ClusterName == profileList[j].ClusterName {
			if profileList[i].id < profileList[j].id {
				return true
			}
		}
		return false
	})

	return profileList, nil
}

/*
Return the names of all available nodes
*/
func (config *NodesConf) ListAllNodes() []string {
	nodeList := make([]string, len(config.nodes))
	for name := range config.nodes {
		nodeList = append(nodeList, name)
	}
	return nodeList
}

/*
Return the names of all available nodes
*/
func (config *NodesConf) ListAllProfiles() []string {
	var nodeList []string
	for name := range config.nodeProfiles {
		nodeList = append(nodeList, name)
	}
	return nodeList
}

/*
FindDiscoverableNode returns the first discoverable node and an
interface to associate with the discovered interface. If the nodUNDEFe has
a primary interface, it is returned; otherwise, the first interface
without a hardware address is returned.

If no unconfigured node is found, an error is returned.
*/
func (config *NodesConf) FindDiscoverableNode() (Node, string, error) {

	nodes, _ := config.FindAllNodes()

	for _, node := range nodes {
		if !(node.Discoverable.Bool()) {
			continue
		}
		if _, ok := node.NetDevs[node.PrimaryNetDev]; ok {
			return node, node.PrimaryNetDev, nil
		}
		for netdev, dev := range node.NetDevs {
			if dev.Hwaddr != "" {
				return node, netdev, nil
			}
		}
	}

	return EmptyNode(), "", ErrNoUnconfigured
}

/*
interface so that nodes and profiles which aren't exported will
be marshaled
*/
type ExportedYml struct {
	WWInternal   int                 `yaml:"WW_INTERNAL"`
	NodeProfiles map[string]*Profile `yaml:"nodeprofiles"`
	Nodes        map[string]*Node    `yaml:"nodes"`
}

/*
Marshall Exported stuff, not NodesConf directly
*/
func (yml *NodesConf) MarshalYAML() (interface{}, error) {
	wwlog.Debug("marshall yml")
	var exp ExportedYml
	exp.WWInternal = yml.WWInternal
	exp.Nodes = yml.nodes
	exp.NodeProfiles = yml.nodeProfiles
	node := yaml.Node{}
	err := node.Encode(exp)
	if err != nil {
		return node, err
	}
	return node, err
}

/*
Unmarshal to intermediate format
*/
func (yml *NodesConf) UnmarshalYAML(
	unmarshal func(interface{}) (err error),
) (err error) {
	wwlog.Debug("UnmarshalYAML called")
	var exp ExportedYml
	err = unmarshal(&exp)
	if err != nil {
		return
	}
	yml.WWInternal = exp.WWInternal
	yml.nodes = exp.Nodes
	yml.nodeProfiles = exp.NodeProfiles
	return nil
}

func (yml NodesConf) IsZero() bool {
	return true
}

/*
Gets a node by its hardware(mac) address
*/
func (config *NodesConf) FindByHwaddr(hwa string) (Node, error) {
	if _, err := net.ParseMAC(hwa); err != nil {
		return Node{}, errors.New("invalid hardware address: " + hwa)
	}
	nodeList, _ := config.FindAllNodes()
	for _, node := range nodeList {
		for _, dev := range node.NetDevs {
			if strings.EqualFold(dev.Hwaddr, hwa) {
				return node, nil
			}
		}
	}

	return Node{}, ErrNotFound
}

/*
Find a node by its ip address
*/
func (config *NodesConf) FindByIpaddr(ipaddr string) (Node, error) {
	addr := net.ParseIP(ipaddr)
	if addr == nil {
		return Node{}, errors.New("invalid IP:" + ipaddr)
	}
	nodeList, err := config.FindAllNodes()
	if err != nil {
		return Node{}, err
	}
	for _, node := range nodeList {
		for _, dev := range node.NetDevs {
			if dev.Ipaddr.Equal(addr) {
				return node, nil
			}
		}
	}

	return Node{}, ErrNotFound
}

/*
struct to hold the fields of GetFields
*/
type NodeFields struct {
	Field  string
	Source string
	Value  string
}

func (f *NodeFields) Set(src, val string) {
	if val == "" {
		return
	}
	if f.Value == "" {
		f.Value = val
		f.Source = src
	} else if f.Source != "" {
		f.Value = val
		if src == "" {
			f.Source = "SUPERSEDED"
		} else {
			f.Source = src
		}
	}

}

type fieldMap map[string]*NodeFields

/*
Get all the info out of Node. If emptyFields is set true, all fields are shown not only the ones with effective values
*/
func (nodeYml *NodesConf) GetFields(node Node) (output []NodeFields) {
	nodeMap := make(fieldMap)
	for _, p := range node.Profiles {
		if profile, ok := nodeYml.nodeProfiles[p]; ok {
			nodeMap.recursiveFields(profile, "", p)
		}
	}
	rawNode, _ := nodeYml.GetNodeOnlyPtr(node.id)
	nodeMap.recursiveFields(rawNode, "", "")
	for _, elem := range nodeMap {
		if elem.Value != "" {
			output = append(output, *elem)
		}
	}
	sort.Slice(output, func(i, j int) bool {
		return output[i].Field < output[j].Field
	})
	return output
}

/*
Get all the info out of Profile. If emptyFields is set true, all fields are shown not only the ones with effective values
*/
func (nodeYml *NodesConf) GetFieldsProfile(profile Profile) (output []NodeFields) {
	profileMap := make(fieldMap)
	profileMap.recursiveFields(&profile, "", "")
	for _, elem := range profileMap {
		if elem.Value != "" {
			output = append(output, *elem)
		}
	}
	sort.Slice(output, func(i, j int) bool {
		return output[i].Field < output[j].Field
	})
	return output
}

/*
Internal function which travels through all fields of a Node and for this
reason needs to be called via interface{}
*/
func (fieldMap fieldMap) recursiveFields(obj interface{}, prefix string, source string) {
	valObj := reflect.ValueOf(obj)
	typeObj := reflect.TypeOf(obj)
	if valObj.IsNil() {
		return
	}
	for i := 0; i < typeObj.Elem().NumField(); i++ {
		if valObj.Elem().Field(i).IsValid() {
			if !typeObj.Elem().Field(i).IsExported() {
				continue
			}
			switch typeObj.Elem().Field(i).Type.Kind() {
			case reflect.Map:
				mapIter := valObj.Elem().Field(i).MapRange()
				for mapIter.Next() {
					fieldMap.recursiveFields(mapIter.Value().Interface(), prefix+typeObj.Elem().Field(i).Name+"["+mapIter.Key().String()+"].", source)
				}
				if valObj.Elem().Field(i).Len() == 0 {
					fieldMap[prefix+typeObj.Elem().Field(i).Name] = &NodeFields{
						Field: prefix + typeObj.Elem().Field(i).Name + "[]",
					}
				}
			case reflect.Struct:
				fieldMap.recursiveFields(valObj.Elem().Field(i).Addr().Interface(), "", source)
			case reflect.Ptr:
				if valObj.Elem().Field(i).Addr().IsValid() {
					fieldMap.recursiveFields(valObj.Elem().Field(i).Interface(), prefix+typeObj.Elem().Field(i).Name+".", source)
				}
			default:
				if _, ok := fieldMap[prefix+typeObj.Elem().Field(i).Name]; !ok {
					fieldMap[prefix+typeObj.Elem().Field(i).Name] = &NodeFields{
						Field:  prefix + typeObj.Elem().Field(i).Name,
						Source: source,
					}
				}

				switch typeObj.Elem().Field(i).Type {
				case reflect.TypeOf([]string{}):
					vals := (valObj.Elem().Field(i).Interface()).([]string)
					fieldMap[prefix+typeObj.Elem().Field(i).Name] = &NodeFields{
						Field:  prefix + typeObj.Elem().Field(i).Name,
						Source: source,
						Value:  strings.Join(vals, ","),
					}
				case reflect.TypeOf(net.IP{}):
					val := (valObj.Elem().Field(i).Interface()).(net.IP)
					if val != nil {
						fieldMap[prefix+typeObj.Elem().Field(i).Name].Set(source, val.String())
					}
				case reflect.TypeOf(net.IPMask{}):
					val := (valObj.Elem().Field(i).Interface()).(net.IPMask)
					if val != nil {
						fieldMap[prefix+typeObj.Elem().Field(i).Name].Set(source, val.String())
					}
				case reflect.TypeOf(true):
					val := (valObj.Elem().Field(i).Interface()).(bool)
					if val {
						fieldMap[prefix+typeObj.Elem().Field(i).Name].Set(source, strconv.FormatBool(val))
					}
				default:
					fieldMap[prefix+typeObj.Elem().Field(i).Name].Set(source, valObj.Elem().Field(i).String())
				}

			} /*else if typeObj.Elem().Field(i).Type.Kind() == reflect.Ptr {
				fieldMap.recursiveFields(valObj.Elem().Field(i).Interface(), emptyFields, prefix+typeObj.Elem().Field(i).Name+".", source)
			}*/
		}
	}
}

/*
Calculate the hash of NodesConf in an orderder fashion
*/
func (config *NodesConf) Hash() [32]byte {
	// flatten out profiles and nodes
	for _, val := range config.nodeProfiles {
		val.Flatten()
	}
	for _, val := range config.nodes {
		val.Flatten()
	}
	data, err := yaml.Marshal(config)
	if err != nil {
		wwlog.Warn("couldn't marshall NodesConf for hashing")
	}
	return sha256.Sum256(data)
}

/*
Return the hash as string
*/
func (config *NodesConf) StringHash() string {
	buffer := config.Hash()
	return hex.EncodeToString(buffer[:])
}
