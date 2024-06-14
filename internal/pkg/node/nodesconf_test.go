package node

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/warewulf/warewulf/internal/pkg/wwlog"
	"gopkg.in/yaml.v3"
)

func newConstructorPrimaryNetworkTest(t *testing.T) NodesConf {
	var data = `
nodeprofiles:
  default:
    comment: This profile is automatically included for each node
  overrideprofile:
    network devices:
      override:
        device: ib0
        type: profile
nodes:
  test_node1:
    network devices:
      net0:
        device: eth0
  test_node2:
    primary network: net1
    network devices:
      net0:
        device: eth0
      net1:
        device: eth1
  test_node3:
    network devices:
      net0:
        device: eth0
      net1:
        device: eth1
  test_node4:
    primary network: net3
    network devices:
      net0:
        device: eth0
      net1:
        device: eth1
  test_node5:
    profiles:
    - overrideprofile
  test_node6:
    profiles:
    - overrideprofile
    network devices:
      override:
        device: ib1
  `
	var ret NodesConf
	err := yaml.Unmarshal([]byte(data), &ret)
	assert.NoError(t, err)
	return ret
}

func Test_Primary_Network(t *testing.T) {
	//wwlog.SetLogLevel(wwlog.DEBUG)
	c := newConstructorPrimaryNetworkTest(t)
	test_node1, err := c.GetNode("test_node1")
	assert.NoError(t, err)
	test_node2, err := c.GetNode("test_node2")
	assert.NoError(t, err)
	test_node3, err := c.GetNode("test_node3")
	assert.NoError(t, err)
	test_node4, err := c.GetNode("test_node4")
	assert.NoError(t, err)
	test_node5, err := c.GetNode("test_node5")
	assert.NoError(t, err)
	test_node6, err := c.GetNode("test_node6")
	assert.NoError(t, err)
	t.Run("Primary network with one network, nothing set", func(t *testing.T) {
		if test_node1.PrimaryNetDev != "net0" {
			t.Errorf("primary network isn't net0 but: %s", test_node1.PrimaryNetDev)
		}
		if !test_node1.NetDevs["net0"].primary {
			t.Errorf("primary flag isn't set for net0")
		}
	})
	t.Run("Primary network with two networks, primary is net1", func(t *testing.T) {
		if test_node2.PrimaryNetDev != "net1" {
			t.Errorf("primary network isn't net1 but: %s", test_node2.PrimaryNetDev)
		}
		if test_node2.NetDevs["net0"].primary {
			t.Errorf("primary flag is set for net0")
		}
		if !test_node2.NetDevs["net1"].primary {
			t.Errorf("primary flag isn't set for net1")
		}
	})
	t.Run("Primary network with two networks, primary isn't set", func(t *testing.T) {
		if test_node3.PrimaryNetDev != "net0" && test_node3.PrimaryNetDev != "net1" {
			t.Errorf("network wasn't sanitized")
		}
		if test_node3.NetDevs["net0"].primary == test_node3.NetDevs["net1"].primary {
			t.Errorf("primary flag isn't set at all")
		}
	})
	// debateable what result we await here, on refactoring primary network w
	// will be one of the valid networks
	t.Run("Primary network with two networks, primary available", func(t *testing.T) {
		if test_node4.PrimaryNetDev == "net3" {
			t.Errorf("primary network isn net3, although node hasn't this network")
		}
		if test_node4.NetDevs["net0"].primary == test_node4.NetDevs["net1"].primary {
			t.Errorf("node primary flag isn't set")
		}
	})
	t.Run("defined in profile", func(t *testing.T) {
		assert.Equal(t, test_node5.NetDevs["override"].Device, "ib0")
		assert.Equal(t, test_node5.NetDevs["override"].Type, "profile")
	})
	t.Run("redefined in profile", func(t *testing.T) {
		assert.Equal(t, test_node6.NetDevs["override"].Device, "ib1")
		assert.Equal(t, test_node6.NetDevs["override"].Type, "profile")
	})
}

var findDiscoverableNodeTests = []struct {
	description          string
	discoverable_nodes   []string
	discovered_node      string
	discovered_interface string
	succeed              bool
}{
	{"no discoverable nodes", []string{}, "", "", false},
	{"all nodes discoverable", []string{"test_node1", "test_node2", "test_node3", "test_node4"}, "test_node1", "net0", true},
	{"discover primary", []string{"test_node2"}, "test_node2", "net1", true},
	{"discovery without primary", []string{"test_node3"}, "test_node3", "net0", true},
}

func Test_FindDiscoverableNode(t *testing.T) {
	for _, tt := range findDiscoverableNodeTests {
		t.Run(tt.description, func(t *testing.T) {
			config := newConstructorPrimaryNetworkTest(t)
			for _, node := range tt.discoverable_nodes {
				config.nodes[node].Discoverable = "true"
			}
			discovered_node, discovered_interface, err := config.FindDiscoverableNode()
			if !tt.succeed {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.discovered_node, discovered_node.Id())
				assert.Equal(t, tt.discovered_interface, discovered_interface)
			}
		})
	}
}

func Test_Profile_Overlay_Merge(t *testing.T) {
	nodesconf := `
nodeprofiles:
  profile1:
    runtime overlay:
      - p1o1
      - p1o2
  profile2:
    runtime overlay:
      - p2o1
      - p2o2
nodes:
  node1:
    profiles:
      - profile1
  node2:
    profiles:
      - profile1
      - profile2
  node3:
    runtime overlay:
      - n3o1
      - n3o2
    profiles:
      - profile1
  node4:
    runtime overlay:
      - n1o1
      - n1o2
    profiles:
      - profile1
      - profile2
  node5:
    runtime overlay:
      - n1o1
      - ~p1o2
    profiles:
      - profile1
      - profile2
`
	assert := assert.New(t)
	var ymlSrc NodesConf
	err := yaml.Unmarshal([]byte(nodesconf), &ymlSrc)
	assert.NoError(err)
	wwlog.SetLogLevel(wwlog.DEBUG)
	nodes, err := ymlSrc.FindAllNodes()
	assert.NoError(err)
	nodemap := make(map[string]*Node)
	for i := range nodes {
		nodemap[nodes[i].Id()] = &nodes[i]
	}
	assert.Contains(nodemap, "node1")

	assert.ElementsMatch(nodemap["node1"].RuntimeOverlay, []string{"p1o1", "p1o2"})
	assert.Contains(nodemap, "node2")
	assert.ElementsMatch(nodemap["node2"].RuntimeOverlay, []string{"p1o1", "p1o2", "p2o1", "p2o2"})
	assert.Contains(nodemap, "node3")
	assert.ElementsMatch(nodemap["node3"].RuntimeOverlay, []string{"p1o1", "p1o2", "n3o1", "n3o2"})
	assert.Contains(nodemap, "node4")
	assert.ElementsMatch(nodemap["node4"].RuntimeOverlay, []string{"p1o1", "p1o2", "p2o1", "p2o2", "n1o1", "n1o2"})
	assert.Contains(nodemap, "node5")
	assert.ElementsMatch(nodemap["node5"].RuntimeOverlay, []string{"p1o1", "p1o2", "~p1o2", "p2o1", "p2o2", "n1o1"})
}

func Test_negated_list(t *testing.T) {
	assert := assert.New(t)
	list := []string{"tok1", "tok2"}
	list2 := []string{"tok1", "tok2", "~tok3"}
	list3 := []string{"tok1", "tok2", "~tok3", "~tok3"}
	list4 := []string{"tok1", "tok2", "~tok3", "~tok4"}
	list5 := []string{"tok1", "tok3", "~tok3", "tok2"}
	assert.Equal([]string{"tok3"}, negList(list2))
	assert.Equal(list, cleanList(list2))
	assert.Equal(list, cleanList(list3))
	assert.Equal(list, cleanList(list4))
	assert.Equal(list, cleanList(list5))
}

func Test_Empty(t *testing.T) {
	var netdev NetDev
	var netdevPtr *NetDev

	t.Run("test for empty", func(t *testing.T) {
		if ObjectIsEmpty(netdev) != true {
			t.Errorf("netdev must be empty")
		}
	})
	t.Run("test for non empty", func(t *testing.T) {
		netdev.Device = "foo"
		if ObjectIsEmpty(netdev) == true {
			t.Errorf("netdev must be non empty")
		}
	})
	t.Run("test for nil pointer", func(t *testing.T) {
		if ObjectIsEmpty(netdevPtr) != true {
			t.Errorf("netdev must be empty")
		}
	})
	/*
		t.Run("test for pointer assigned", func(t *testing.T) {
			netdev.Ipaddr = net.ParseIP("10.10.10.1")
			netdevPtr = &netdev
			if ObjectIsEmpty(netdevPtr) == true {
				t.Errorf("netdev must be empty")
			}
		})
	*/
}

func TestHash(t *testing.T) {
	nodeConfYml1 := `
WW_INTERNAL: 45
nodeprofiles:
  default:
    comment: This profile is automatically included for each node
  test:
    comment: This is a test profile
nodes:
  n01:
    discoverable: true
    profiles:
    - default
    network devices:
      default:
        ipaddr: 10.0.10.1
  n02:
    discoverable: true
    profiles:
    - default
    network devices:
      default:
        ipaddr: 10.0.10.2
`
	nodeConfYml2 := `
WW_INTERNAL: 45
nodeprofiles:
  default:
    comment: This profile is automatically included for each node
  test:
    comment: This is a test profile
nodes:
  n02:
    discoverable: true
    profiles:
    - default
    network devices:
      default:
        ipaddr: 10.0.10.2
  n01:
    discoverable: true
    profiles:
    - default
    network devices:
      default:
        ipaddr: 10.0.10.1
`
	nodeConfYml3 := `
WW_INTERNAL: 45
nodeprofiles:
  default:
    comment: This profile is automatically included for each node
  test:
    comment: This is a test profile
nodes:
  n02:
    discoverable: true
    profiles:
    - default
    network devices:
      default:
        ipaddr: 10.0.10.2
  n01:
    discoverable: true
    profiles:
    - default
    network devices:
      default:
        ipaddr: 10.0.10.3
`
	var nodeConf1, nodeConf2, nodeConf3 NodesConf
	err := yaml.Unmarshal([]byte(nodeConfYml1), &nodeConf1)
	assert.NoError(t, err)
	err = yaml.Unmarshal([]byte(nodeConfYml2), &nodeConf2)
	assert.NoError(t, err)
	err = yaml.Unmarshal([]byte(nodeConfYml3), &nodeConf3)
	assert.NoError(t, err)

	t.Run("Same NodesConf with same conf", func(t *testing.T) {
		var testConf NodesConf
		err = yaml.Unmarshal([]byte(nodeConfYml1), &testConf)
		assert.NoError(t, err)
		if testConf.Hash() != nodeConf1.Hash() {
			err = yaml.Unmarshal([]byte(nodeConfYml1), nodeConf1)
			assert.NoError(t, err)
			t.Errorf("Hashes for same configuration differs: %x != %x", nodeConf1.Hash(), nodeConf1.Hash())
		}
	})

	t.Run("Different sorted NodesConf with same conf", func(t *testing.T) {
		yml1, err := yaml.Marshal(nodeConf1)
		assert.NoError(t, err)
		yml2, err := yaml.Marshal(nodeConf2)
		assert.NoError(t, err)
		if nodeConf2.Hash() != nodeConf1.Hash() {
			t.Errorf("Hashes for same configuration differs: %x != %x\njson1:\n%s,yml2:\n%s\n", nodeConf2.Hash(), nodeConf1.Hash(), yml1, yml2)
		}
	})

	t.Run("Different NodesConf with different conf", func(t *testing.T) {
		if nodeConf2.Hash() == nodeConf3.Hash() {
			t.Errorf("Hashes for different configuration is the same: %x == %x", nodeConf2.Hash(), nodeConf3.Hash())
		}
	})
}
