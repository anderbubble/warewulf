package node

import (
	"fmt"
	"net"
	"reflect"
	"strconv"
	"strings"

	"github.com/hpcng/warewulf/internal/pkg/util"
	"github.com/spf13/cobra"
)

/*
Create cmd line flags from the NodeConf fields. Returns a []func() where every function
must be called, as the commandline parser returns e.g. netip.IP objects which must be parsed
back to strings. Gives back error if string can't be parsed to desired type. Valid types are
"bool","IP","MAC"
*/
func (nodeConf *NodeConf) CreateFlags(baseCmd *cobra.Command, excludeList []string) (converters []func() error) {
	nodeInfoType := reflect.TypeOf(nodeConf)
	nodeInfoVal := reflect.ValueOf(nodeConf)
	// now iterate of every field
	for i := 0; i < nodeInfoVal.Elem().NumField(); i++ {
		if nodeInfoType.Elem().Field(i).Tag.Get("comment") != "" &&
			!util.InSlice(excludeList, nodeInfoType.Elem().Field(i).Tag.Get("lopt")) {
			field := nodeInfoVal.Elem().Field(i)
			converters = append(converters, createFlags(baseCmd, excludeList, nodeInfoType.Elem().Field(i), &field)...)
		} else if nodeInfoType.Elem().Field(i).Type.Kind() == reflect.Ptr {
			nestType := reflect.TypeOf(nodeInfoVal.Elem().Field(i).Interface())
			nestVal := reflect.ValueOf(nodeInfoVal.Elem().Field(i).Interface())
			for j := 0; j < nestType.Elem().NumField(); j++ {
				field := nestVal.Elem().Field(j)
				converters = append(converters, createFlags(baseCmd, excludeList, nestType.Elem().Field(j), &field)...)
			}
		} else if nodeInfoType.Elem().Field(i).Type == reflect.TypeOf(map[string]*NetDevs(nil)) {
			netMap := nodeInfoVal.Elem().Field(i).Interface().(map[string]*NetDevs)
			// add a default network so that it can hold values
			key := "default"
			if len(netMap) == 0 {
				netMap[key] = new(NetDevs)
			} else {
				for keyIt := range netMap {
					key = keyIt
					break
				}
			}
			netType := reflect.TypeOf(netMap[key])
			netVal := reflect.ValueOf(netMap[key])
			for j := 0; j < netType.Elem().NumField(); j++ {
				field := netVal.Elem().Field(j)
				converters = append(converters, createFlags(baseCmd, excludeList, netType.Elem().Field(j), &field)...)
			}
		}
	}
	return converters
}

/*
Helper function to create the different PerisitantFlags() for different types.
*/
func createFlags(baseCmd *cobra.Command, excludeList []string,
	myType reflect.StructField, myVal *reflect.Value) (converters []func() error) {
	if myType.Tag.Get("lopt") != "" {
		if myType.Type.Kind() == reflect.String {
			switch myType.Tag.Get("type") {
			case "uint":
				converters = append(converters, createUintFlags(baseCmd, excludeList, myType, myVal))
			case "bool":
				converters = append(converters, createBoolFlags(baseCmd, excludeList, myType, myVal))
			case "IP":
				converters = append(converters, createIPFlags(baseCmd, excludeList, myType, myVal))
			case "IPMask":
				converters = append(converters, createIPMaskFlags(baseCmd, excludeList, myType, myVal))
			case "MAC":
				converters = append(converters, createMACFlags(baseCmd, excludeList, myType, myVal))
			default:
				createStringFlags(baseCmd, excludeList, myType, myVal)
			}
		} else if myType.Type == reflect.TypeOf([]string{}) {
			createStringListFlags(baseCmd, excludeList, myType, myVal)
		} else if myType.Type == reflect.TypeOf(map[string]string{}) {
			createStringMapFlags(baseCmd, excludeList, myType, myVal)
		}
	}
	return converters
}


func createUintFlags(baseCmd *cobra.Command, excludeList []string,
	myType reflect.StructField, myVal *reflect.Value) (func() error) {
	
	ptr := myVal.Addr().Interface().(*string)

	if myType.Tag.Get("sopt") != "" {
		baseCmd.PersistentFlags().StringVarP(ptr,
			myType.Tag.Get("lopt"),
			myType.Tag.Get("sopt"),
			myType.Tag.Get("default"),
			myType.Tag.Get("comment"))
	} else {
		baseCmd.PersistentFlags().StringVar(ptr,
			myType.Tag.Get("lopt"),
			myType.Tag.Get("default"),
			myType.Tag.Get("comment"))
	}
	
	return func() error {
		if !util.InSlice(GetUnsetVerbs(), *ptr) && *ptr != "" {
			_, err := strconv.ParseUint(myType.Tag.Get(*ptr), 10, 32)
			if err != nil {
				return err
			}
		}
		return nil
	}
}


func createBoolFlags(baseCmd *cobra.Command, excludeList []string,
	/* Can't use the bool var from pflag as we need the UNSET
	   verbs to be passed correctly */
	myType reflect.StructField, myVal *reflect.Value) (func() error) {

	ptr := myVal.Addr().Interface().(*string)

	if myType.Tag.Get("sopt") != "" {
		baseCmd.PersistentFlags().StringVarP(ptr,
			myType.Tag.Get("lopt"),
			myType.Tag.Get("sopt"),
			"",
			myType.Tag.Get("comment"))
	} else {
		baseCmd.PersistentFlags().StringVar(ptr,
			myType.Tag.Get("lopt"),
			"",
			myType.Tag.Get("comment"))
	}
	baseCmd.PersistentFlags().Lookup(myType.Tag.Get("lopt")).NoOptDefVal = "true"

	return func() error {
		if !util.InSlice(GetUnsetVerbs(), *ptr) && *ptr != "" {
			if strings.ToLower(*ptr) != "yes" {
				*ptr = "true"
				return nil
			}
			if strings.ToLower(*ptr) != "no" {
				*ptr = "false"
				return nil
			}
			val, err := strconv.ParseBool(*ptr)
			if err != nil {
				return fmt.Errorf("commandline option %s needs to be bool", myType.Tag.Get("lopt"))
			}
			*ptr = strconv.FormatBool(val)
		}
		return nil
	}
}


func createIPFlags(baseCmd *cobra.Command, excludeList []string,
	myType reflect.StructField, myVal *reflect.Value) (func() error) {

	ptr := myVal.Addr().Interface().(*string)
	var valueRaw net.IP

	defaultConv := net.ParseIP(myType.Tag.Get("default"))
	if myType.Tag.Get("sopt") != "" {
		baseCmd.PersistentFlags().IPVarP(&valueRaw,
			myType.Tag.Get("lopt"),
			myType.Tag.Get("sopt"),
			defaultConv,
			myType.Tag.Get("comment"))
	} else {
		baseCmd.PersistentFlags().IPVar(&valueRaw,
			myType.Tag.Get("lopt"),
			defaultConv,
			myType.Tag.Get("comment"))
	}

	return func() error {
		if util.InSlice(GetUnsetVerbs(), *ptr) && *ptr != "" {
		}
		if valueRaw != nil {
			// will always get a IP, not a string
			*ptr = valueRaw.String()
		}
		return nil
	}
}


func createIPMaskFlags(baseCmd *cobra.Command, excludeList []string,
	myType reflect.StructField, myVal *reflect.Value) (func() error) {

	ptr := myVal.Addr().Interface().(*string)
	var valueRaw net.IPMask

	defaultConv := net.ParseIP(myType.Tag.Get("default")).DefaultMask()
	if myType.Tag.Get("sopt") != "" {
		baseCmd.PersistentFlags().IPMaskVarP(&valueRaw,
			myType.Tag.Get("lopt"),
			myType.Tag.Get("sopt"),
			defaultConv,
			myType.Tag.Get("comment"))
	} else {
		baseCmd.PersistentFlags().IPMaskVar(&valueRaw,
			myType.Tag.Get("lopt"),
			defaultConv,
			myType.Tag.Get("comment"))
	}

	return 	func() error {
		if valueRaw != nil {
			*ptr = valueRaw.String()
			return nil
		} else {
			return fmt.Errorf("could not parse %s to IP", valueRaw.String())
		}
	}
}


func createMACFlags(baseCmd *cobra.Command, excludeList []string,
	myType reflect.StructField, myVal *reflect.Value) (func() error) {

	ptr := myVal.Addr().Interface().(*string)

	if myType.Tag.Get("sopt") != "" {
		baseCmd.PersistentFlags().StringVarP(ptr,
			myType.Tag.Get("lopt"),
			myType.Tag.Get("sopt"),
			"",
			myType.Tag.Get("comment"))
	} else {
		baseCmd.PersistentFlags().StringVar(ptr,
			myType.Tag.Get("lopt"),
			"",
			myType.Tag.Get("comment"))
	}

	return func() error {
		myMac, err := net.ParseMAC(*ptr)
		if err != nil {
			return err
		}
		*ptr = myMac.String()
		return nil
	}
}


func createStringFlags(baseCmd *cobra.Command, excludeList []string,
	myType reflect.StructField, myVal *reflect.Value) {

	ptr := myVal.Addr().Interface().(*string)

	if myType.Tag.Get("sopt") != "" {
		baseCmd.PersistentFlags().StringVarP(ptr,
			myType.Tag.Get("lopt"),
			myType.Tag.Get("sopt"),
			myType.Tag.Get("default"),
			myType.Tag.Get("comment"))
	} else {
		baseCmd.PersistentFlags().StringVar(ptr,
			myType.Tag.Get("lopt"),
			myType.Tag.Get("default"),
			myType.Tag.Get("comment"))
	}
}


func createStringListFlags(baseCmd *cobra.Command, excludeList []string,
	myType reflect.StructField, myVal *reflect.Value) {

	ptr := myVal.Addr().Interface().(*[]string)

	if myType.Tag.Get("sopt") != "" {
		baseCmd.PersistentFlags().StringSliceVarP(ptr,
			myType.Tag.Get("lopt"),
			myType.Tag.Get("sopt"),
			[]string{myType.Tag.Get("default")},
			myType.Tag.Get("comment"))
	} else if !util.InSlice(excludeList, myType.Tag.Get("lopt")) {
		baseCmd.PersistentFlags().StringSliceVar(ptr,
			myType.Tag.Get("lopt"),
			[]string{myType.Tag.Get("default")},
			myType.Tag.Get("comment"))	
	}
}


func createStringMapFlags(baseCmd *cobra.Command, excludeList []string,
	myType reflect.StructField, myVal *reflect.Value) {

	ptr := myVal.Addr().Interface().(*map[string]string)

	if myType.Tag.Get("sopt") != "" {
		baseCmd.PersistentFlags().StringToStringVarP(ptr,
			myType.Tag.Get("lopt"),
			myType.Tag.Get("sopt"),
			map[string]string{}, // empty default!
			myType.Tag.Get("comment"))
	} else if !util.InSlice(excludeList, myType.Tag.Get("lopt")) {
		baseCmd.PersistentFlags().StringToStringVar(ptr,
			myType.Tag.Get("lopt"),
			map[string]string{}, // empty default!
			myType.Tag.Get("comment"))
	}
}
