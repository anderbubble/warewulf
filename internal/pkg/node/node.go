package node

import (
	"fmt"
	"net"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/warewulf/warewulf/internal/pkg/util"
	"github.com/warewulf/warewulf/internal/pkg/wwtype"
)

/*
Node is the datastructure describing a node and a profile which in disk format.
*/
type Node struct {
	id    string
	valid bool // Is set true, if called by the constructor
	// exported values
	Discoverable wwtype.WWbool     `yaml:"discoverable,omitempty" lopt:"discoverable" sopt:"e" comment:"Make discoverable in given network (true/false)"`
	AssetKey     string            `yaml:"asset key,omitempty" lopt:"asset" comment:"Set the node's Asset tag (key)"`
	Profiles     []string          `yaml:"profiles,omitempty" lopt:"profile" sopt:"P" comment:"Set the node's profile members (comma separated)"`
	Profile      `yaml:"-,inline"` // include all values set in the profile, but inline them in yaml output if these are part of Node
}

/*
Creates an Node with the given id. Doesn't add it to the database
*/
func NewNode(id string) (node Node) {
	node = EmptyNode()
	node.id = id
	fmt.Printf("New node with id: %s", node.id)
	return node
}

func EmptyNode() (node Node) {
	node.Ipmi = new(IPMI)
	node.Ipmi.Tags = map[string]string{}
	node.Kernel = new(Kernel)
	node.NetDevs = make(map[string]*NetDev)
	node.Tags = map[string]string{}
	return node
}

/*
Returns the id of the node
*/
func (node *Node) Id() string {
	return node.id
}

/*
Returns if the node is a valid in the database
*/
func (node *Node) Valid() bool {
	return node.valid
}

/*
Flattens out a Node, which means if there are no explicit values in *IPMI
or *Kernel, these pointer will set to nil. This will remove something like
ipmi: {} from nodes.conf
*/
func (info *Node) Flatten() {
	recursiveFlatten(info)
}

/*
Checks if for Node all values can be parsed according to their type.
*/
func (node *Node) Check() (err error) {
	nodeInfoType := reflect.TypeOf(node)
	nodeInfoVal := reflect.ValueOf(node)
	// now iterate of every field
	for i := 0; i < nodeInfoVal.Elem().NumField(); i++ {
		//wwlog.Debug("checking field: %s type: %s", nodeInfoType.Elem().Field(i).Name, nodeInfoVal.Elem().Field(i).Type())
		if nodeInfoType.Elem().Field(i).Type.Kind() == reflect.String {
			newFmt, err := checker(nodeInfoVal.Elem().Field(i).Interface().(string), nodeInfoType.Elem().Field(i).Tag.Get("type"))
			if err != nil {
				return fmt.Errorf("field: %s value:%s err: %s", nodeInfoType.Elem().Field(i).Name, nodeInfoVal.Elem().Field(i).String(), err)
			} else if newFmt != "" {
				nodeInfoVal.Elem().Field(i).SetString(newFmt)
			}
		} else if nodeInfoType.Elem().Field(i).Type.Kind() == reflect.Ptr && !nodeInfoVal.Elem().Field(i).IsNil() {
			nestType := reflect.TypeOf(nodeInfoVal.Elem().Field(i).Interface())
			nestVal := reflect.ValueOf(nodeInfoVal.Elem().Field(i).Interface())
			for j := 0; j < nestType.Elem().NumField(); j++ {
				if nestType.Elem().Field(j).Type.Kind() == reflect.String {
					//wwlog.Debug("checking field: %s type: %s", nestType.Elem().Field(j).Name, nestType.Elem().Field(j).Tag.Get("type"))
					newFmt, err := checker(nestVal.Elem().Field(j).Interface().(string), nestType.Elem().Field(j).Tag.Get("type"))
					if err != nil {
						return fmt.Errorf("field: %s value:%s err: %s", nestType.Elem().Field(j).Name, nestVal.Elem().Field(j).String(), err)
					} else if newFmt != "" {
						nestVal.Elem().Field(j).SetString(newFmt)
					}
				}
			}
		} else if nodeInfoType.Elem().Field(i).Type == reflect.TypeOf(map[string]*NetDev(nil)) {
			netMap := nodeInfoVal.Elem().Field(i).Interface().(map[string]*NetDev)
			for _, val := range netMap {
				netType := reflect.TypeOf(val)
				netVal := reflect.ValueOf(val)
				for j := 0; j < netType.Elem().NumField(); j++ {
					newFmt, err := checker(netVal.Elem().Field(j).String(), netType.Elem().Field(j).Tag.Get("type"))
					if err != nil {
						return fmt.Errorf("field: %s value:%s err: %s", netType.Elem().Field(j).Name, netVal.Elem().Field(j).String(), err)
					} else if newFmt != "" {
						netVal.Elem().Field(j).SetString(newFmt)
					}
				}
			}
		}
	}
	return nil
}

func checker(value string, valType string) (niceValue string, err error) {
	if valType == "" || value == "" || util.InSlice(wwtype.GetUnsetVerbs(), value) {
		return "", nil
	}
	//wwlog.Debug("checker: %s is %s", value, valType)
	switch valType {
	case "":
		return "", nil
	case "bool":
		if strings.ToLower(value) == "yes" {
			return "true", nil
		}
		if strings.ToLower(value) == "no" {
			return "false", nil
		}
		myBool, err := strconv.ParseBool(value)
		return strconv.FormatBool(myBool), err
	case "IP":
		if addr := net.ParseIP(value); addr == nil {
			return "", fmt.Errorf("%s can't be parsed to ip address", value)
		} else {
			return addr.String(), nil
		}
	case "MAC":
		if mac, err := net.ParseMAC(value); err != nil {
			return "", fmt.Errorf("%s can't be parsed to MAC address: %s", value, err)
		} else {
			return mac.String(), nil
		}
	case "uint":
		if _, err := strconv.ParseUint(value, 10, 64); err != nil {
			return "", fmt.Errorf("%s is not a uint: %s", value, err)
		}
	}
	return "", nil
}

/*
Filter a given slice of Node against a given
regular expression
*/
func FilterByName(set []Node, searchList []string) []Node {
	var ret []Node
	unique := make(map[string]Node)

	if len(searchList) > 0 {
		for _, search := range searchList {
			for _, entry := range set {
				if match, _ := regexp.MatchString("^"+search+"$", entry.id); match {
					unique[entry.id] = entry
				}
			}
		}
		for _, n := range unique {
			ret = append(ret, n)
		}
	} else {
		ret = set
	}

	sort.Sort(sortByName(ret))
	return ret
}

type sortByName []Node

func (a sortByName) Len() int           { return len(a) }
func (a sortByName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a sortByName) Less(i, j int) bool { return a[i].id < a[j].id }
