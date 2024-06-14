package node

import (
	"net"
	"reflect"
	"strings"

	"github.com/warewulf/warewulf/internal/pkg/util"
)

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
