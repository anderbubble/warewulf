package node

import (
	"fmt"
	"net"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/warewulf/warewulf/internal/pkg/util"
)

type sortByName []Node

func (a sortByName) Len() int           { return len(a) }
func (a sortByName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a sortByName) Less(i, j int) bool { return a[i].id < a[j].id }

/**********
 *
 * Filters
 *
 *********/

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

/*
Filter a given map of Node against given regular expression.
*/
func FilterNodesByName(inputMap map[string]*Node, searchList []string) (retMap map[string]*Node) {
	retMap = map[string]*Node{}
	if len(searchList) > 0 {
		for _, search := range searchList {
			for name, nConf := range inputMap {
				if match, _ := regexp.MatchString("^"+search+"$", name); match {
					retMap[name] = nConf
				}
			}
		}
	}
	return retMap
}

/*
Filter a given map of Node against given regular expression.
*/
func FilterProfilesByName(inputMap map[string]*Profile, searchList []string) (retMap map[string]*Profile) {
	retMap = map[string]*Profile{}
	if len(searchList) > 0 {
		for _, search := range searchList {
			for name, nConf := range inputMap {
				if match, _ := regexp.MatchString("^"+search+"$", name); match {
					retMap[name] = nConf
				}
			}
		}
	}
	return retMap
}

/*
Creates an Node with the given id. Doesn't add it to the database
*/
func NewNode(id string) (node Node) {
	node.Ipmi = new(IPMI)
	node.Ipmi.Tags = map[string]string{}
	node.Kernel = new(Kernel)
	node.NetDevs = make(map[string]*NetDev)
	node.Tags = map[string]string{}
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
Creates a Profile with the given id. Doesn't add it to the database.
*/
func NewProfile(id string) (profileconf Profile) {
	profileconf.Ipmi = new(IPMI)
	profileconf.Ipmi.Tags = map[string]string{}
	profileconf.Kernel = new(Kernel)
	profileconf.NetDevs = make(map[string]*NetDev)
	profileconf.Tags = map[string]string{}
	return profileconf
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
Flattens out a Profile, which means if there are no explicit values in *IPMI
or *Kernel, these pointer will set to nil. This will remove something like
ipmi: {} from nodes.conf
*/
func (info *Profile) Flatten() {
	recursiveFlatten(info)
}

func recursiveFlatten(obj interface{}) (hasContent bool) {
	valObj := reflect.ValueOf(obj)
	typeObj := reflect.TypeOf(obj)
	hasContent = false
	if valObj.IsNil() {
		return
	}
	for i := 0; i < typeObj.Elem().NumField(); i++ {
		if valObj.Elem().Field(i).IsValid() {
			if !typeObj.Elem().Field(i).IsExported() {
				continue
			}
		}
		switch typeObj.Elem().Field(i).Type.Kind() {
		case reflect.Map:
			mapIter := valObj.Elem().Field(i).MapRange()
			for mapIter.Next() {
				if mapIter.Value().Kind() == reflect.String {
					if mapIter.Value().String() != "" {
						// fmt.Println("map")
						hasContent = true
					}
				} else {
					ret := recursiveFlatten(mapIter.Value().Interface())
					hasContent = ret || hasContent
				}
			}

		case reflect.Ptr:
			if valObj.Elem().Field(i).Addr().IsValid() {
				// fmt.Printf("calling: %s with: %v\n", typeObj.Elem().Field(i).Name, hasContent)
				ret := recursiveFlatten((valObj.Elem().Field(i).Interface()))
				if !ret {
					valObj.Elem().Field(i).Set(reflect.Zero(valObj.Elem().Field(i).Type()))
				}
				hasContent = ret || hasContent

			}
			// fmt.Printf("called: %s returned: %v\n", typeObj.Elem().Field(i).Name, hasContent)
		case reflect.Struct:
			ret := recursiveFlatten((valObj.Elem().Field(i).Addr().Interface()))
			hasContent = ret || hasContent
		case reflect.Slice:
			if typeObj.Elem().Field(i).Type == reflect.TypeOf([]string{}) {
				del := false
				for _, elem := range (valObj.Elem().Field(i).Interface()).([]string) {
					if strings.EqualFold(elem, undef) {
						del = true
					}
				}
				if del {
					valObj.Elem().Field(i).SetLen(0)
				}
			}
			if valObj.Elem().Field(i).Len() > 0 {
				// fmt.Println("len")
				hasContent = true
			}
		case reflect.String:
			if strings.EqualFold(valObj.Elem().Field(i).String(), undef) {
				valObj.Elem().Field(i).SetString("")
			}
			if valObj.Elem().Field(i).String() != "" {
				// fmt.Println("string", valObj.Elem().Field(i).String())
				hasContent = true
			}
		case reflect.Bool:
			val := valObj.Elem().Field(i).Interface().(bool)
			hasContent = hasContent || val
		default:
			switch valObj.Elem().Field(i).Type() {
			case reflect.TypeOf(net.IP{}):
				val := valObj.Elem().Field(i).Interface().(net.IP)
				if len(val) != 0 && !val.IsUnspecified() {
					// fmt.Println("IP")
					hasContent = true
				}
			case reflect.TypeOf(net.IPMask{}):
				val := valObj.Elem().Field(i).Interface().(net.IP)
				if len(val) != 0 && !val.IsUnspecified() {
					// fmt.Println("Mask")
					hasContent = true
				}
			default:
			}
		}
		if !hasContent {
			valObj.Elem().Field(i).Set(reflect.Zero(valObj.Elem().Field(i).Type()))
		}
	}
	return
}

/*
Create a string slice, where every element represents a yaml entry, used for node/profile edit
in order to get a summary of all available elements
*/
func UnmarshalConf(obj interface{}, excludeList []string) (lines []string) {
	objType := reflect.TypeOf(obj)
	// now iterate of every field
	for i := 0; i < objType.NumField(); i++ {
		if objType.Field(i).Tag.Get("comment") != "" {
			if ymlStr, ok := getYamlString(objType.Field(i), excludeList); ok {
				lines = append(lines, ymlStr...)
			}
		}
		if objType.Field(i).Type.Kind() == reflect.Ptr && objType.Field(i).Tag.Get("yaml") != "" {
			typeLine := objType.Field(i).Tag.Get("yaml")
			if len(strings.Split(typeLine, ",")) > 1 {
				typeLine = strings.Split(typeLine, ",")[0] + ":"
			}
			lines = append(lines, typeLine)
			nestedLine := UnmarshalConf(reflect.New(objType.Field(i).Type.Elem()).Elem().Interface(), excludeList)
			for _, ln := range nestedLine {
				lines = append(lines, "  "+ln)
			}
		} else if objType.Field(i).Type.Kind() == reflect.Map && objType.Field(i).Type.Elem().Kind() == reflect.Ptr {
			typeLine := objType.Field(i).Tag.Get("yaml")
			if len(strings.Split(typeLine, ",")) > 1 {
				typeLine = strings.Split(typeLine, ",")[0] + ":"
			}
			lines = append(lines, typeLine, "  element:")
			nestedLine := UnmarshalConf(reflect.New(objType.Field(i).Type.Elem().Elem()).Elem().Interface(), excludeList)
			for _, ln := range nestedLine {
				lines = append(lines, "    "+ln)
			}
		}
	}
	return lines
}

/*
Get the string of the yaml tag
*/
func getYamlString(myType reflect.StructField, excludeList []string) ([]string, bool) {
	ymlStr := myType.Tag.Get("yaml")
	if len(strings.Split(ymlStr, ",")) > 1 {
		ymlStr = strings.Split(ymlStr, ",")[0]
	}
	if util.InSlice(excludeList, ymlStr) {
		return []string{""}, false
	} else if myType.Tag.Get("comment") == "" && myType.Type.Kind() == reflect.String {
		return []string{""}, false
	}
	if myType.Type.Kind() == reflect.String {
		fieldType := myType.Tag.Get("type")
		if fieldType == "" {
			fieldType = "string"
		}
		ymlStr += ": " + fieldType
		return []string{ymlStr}, true
	} else if myType.Type == reflect.TypeOf([]string{}) {
		return []string{ymlStr + ":", "  - string"}, true
	} else if myType.Type == reflect.TypeOf(map[string]string{}) {
		return []string{ymlStr + ":", "  key: value"}, true
	} else if myType.Type.Kind() == reflect.Ptr {
		return []string{ymlStr + ":"}, true
	}
	return []string{ymlStr}, true
}

/*
Getters for unexported fields
*/

/*
Returns the id of the node
*/
func (node *Node) Id() string {
	return node.id
}

/*
Returns the id of the profile
*/
func (node *Profile) Id() string {
	return node.id
}

/*
Returns if the node is a valid in the database
*/
func (node *Node) Valid() bool {
	return node.valid
}

/*
Check if the netdev is the primary one
*/
func (dev *NetDev) Primary() bool {
	return dev.primary
}

// returns all negated elements which are marked with ! as prefix
// from a list
func negList(list []string) (ret []string) {
	for _, tok := range list {
		if strings.HasPrefix(tok, "~") {
			ret = append(ret, tok[1:])
		}
	}
	return
}

// clean a list from negated tokens
func cleanList(list []string) (ret []string) {
	neg := negList(list)
	for _, listTok := range list {
		notNegate := true
		for _, negTok := range neg {
			if listTok == negTok || listTok == "~"+negTok {
				notNegate = false
			}
		}
		if notNegate {
			ret = append(ret, listTok)
		}
	}
	return ret
}
