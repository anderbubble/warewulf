package node

import (
	"reflect"
	"strconv"
	"testing"

	"gopkg.in/yaml.v2"
)

func NewTransformerTestNode() NodeYaml {
	var data = `
nodeprofiles:
  default:
    comment: This profile is automatically included for each node
    ipmi:
      username: greg
nodes:
  test_node1:
    comment: Node Comment
    profiles:
    - default
    network devices:
      net0:
        device: eth1
    discoverable: true
    ipmi:
      username: chris
  test_node2:
    profiles:
    - default
    network devices:
      net0:
        netmask: 1.1.1.1
      net1:
        primary: true
`
	var ret NodeYaml
	_ = yaml.Unmarshal([]byte(data), &ret)
	return ret
}
func Test_nodeYaml_SetFrom(t *testing.T) {
	c := NewTransformerTestNode()
	nodes, _ := c.FindAllNodes()
	test_node1 := NewInfo()
	test_node2 := NewInfo()
	for _, n := range nodes {
		if n.Id.Get() == "test_node1" {
			test_node1 = n
		}
		if n.Id.Get() == "test_node2" {
			test_node2 = n
		}
	}
	getByNametests := []struct {
		name    string
		arg     string
		want    string
		wantErr bool
	}{
		{"GetByName: FieldValue", "Comment", "Node Comment", false},
		{"GetByName: FieldName", "comment", "NodeComment", true},
	}
	for _, tt := range getByNametests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetByName(&test_node1, tt.arg)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetByName(%s,%s) error = %v, wantErr %v",
					reflect.TypeOf(test_node1), tt.arg, err, tt.wantErr)
				return
			}
			if (got != tt.want) != tt.wantErr {
				t.Errorf("GetByName(%s,%s) got = %v, want = %v",
					reflect.TypeOf(test_node1), tt.arg, got, tt.want)
				return
			}
		})
	}
	t.Run("Get() comment", func(t *testing.T) {
		comment := test_node1.Comment.Get()
		if comment != "Node Comment" {
			t.Errorf("Get() returned wrong comment: %s", comment)
		}
	})
	t.Run("Get() profile comment", func(t *testing.T) {
		comment := test_node2.Comment.Get()
		if comment != "This profile is automatically included for each node" {
			t.Errorf("Get() returned wrong comment: %s", comment)
		}
	})
	t.Run("Get() default ipxe", func(t *testing.T) {
		value := test_node1.Ipxe.Get()
		if value != "default" {
			t.Errorf("Get() returned wrong ipxe template: %s", value)
		}
	})
	t.Run("GetSlice() default profile", func(t *testing.T) {
		value := test_node1.Profiles.GetSlice()[0]
		if value != "default" {
			t.Errorf("GetSlice() returned wrong profile: %s", value)
		}
	})
	t.Run("Get() default kernel args", func(t *testing.T) {
		value := test_node1.Kernel.Args.Get()
		if value != "quiet crashkernel=no vga=791 net.naming-scheme=v238" {
			t.Errorf("Get() returned wrong kernel args: %s", value)
		}
	})
	t.Run("Get() default network mask", func(t *testing.T) {
		value := test_node1.NetDevs["net0"].Netmask.Get()
		if value != "255.255.255.0" {
			t.Errorf("Get() returned wrong default netmask: %s", value)
		}
	})
	t.Run("Get() default network mask", func(t *testing.T) {
		value := test_node2.NetDevs["net0"].Netmask.Get()
		if value != "1.1.1.1" {
			t.Errorf("Get() returned wrong default netmask: %s", value)
		}
	})
	t.Run("GetB() primary", func(t *testing.T) {
		value := test_node1.NetDevs["net0"].Primary.GetB()
		if !value {
			t.Errorf("GetB() returned wrong: %s", strconv.FormatBool(value))
		}
	})
	t.Run("GetB() primary", func(t *testing.T) {
		value := test_node2.NetDevs["net0"].Primary.GetB()
		if value {
			t.Errorf("GetB() returned wrong: %s", strconv.FormatBool(value))
		}
	})
	t.Run("GetB() primary", func(t *testing.T) {
		value := test_node2.NetDevs["net1"].Primary.GetB()
		if !value {
			t.Errorf("GetB() returned wrong: %s", strconv.FormatBool(value))
		}
	})
	t.Run("GetB() default discoverable", func(t *testing.T) {
		value := test_node1.Discoverable.GetB()
		if !value {
			t.Errorf("GetB() returned wrong: %s", strconv.FormatBool(value))
		}
	})
	t.Run("GetB() default discoverable", func(t *testing.T) {
		value := test_node2.Discoverable.GetB()
		if value {
			t.Errorf("GetB() returned wrong: %s", strconv.FormatBool(value))
		}
	})
	t.Run("Get() ipmi user from profile", func(t *testing.T) {
		value := test_node2.Ipmi.UserName.Get()
		if value != "greg" {
			t.Errorf("Get() returned wrong ipmi username: %s", value)
		}
	})
	t.Run("Get() ipmi user from profile", func(t *testing.T) {
		value := test_node1.Ipmi.UserName.Get()
		if value != "chris" {
			t.Errorf("Get() returned wrong ipmi username: %s", value)
		}
	})
}
