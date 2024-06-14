package node

/*
Holds the data which can be set for profiles and nodes.
*/
type Profile struct {
	id string
	// exported values
	Comment        string                 `yaml:"comment,omitempty" lopt:"comment" comment:"Set arbitrary string comment"`
	ClusterName    string                 `yaml:"cluster name,omitempty" lopt:"cluster" sopt:"c" comment:"Set cluster group"`
	ContainerName  string                 `yaml:"container name,omitempty" lopt:"container" sopt:"C" comment:"Set container name"`
	Ipxe           string                 `yaml:"ipxe template,omitempty" lopt:"ipxe" comment:"Set the iPXE template name"`
	RuntimeOverlay []string               `yaml:"runtime overlay,omitempty" lopt:"runtime" sopt:"R" comment:"Set the runtime overlay"`
	SystemOverlay  []string               `yaml:"system overlay,omitempty" lopt:"wwinit" sopt:"O" comment:"Set the system overlay"`
	Kernel         *Kernel                `yaml:"kernel,omitempty"`
	Ipmi           *IPMI                  `yaml:"ipmi,omitempty"`
	Init           string                 `yaml:"init,omitempty" lopt:"init" sopt:"i" comment:"Define the init process to boot the container"`
	Root           string                 `yaml:"root,omitempty" lopt:"root" comment:"Define the rootfs" `
	NetDevs        map[string]*NetDev     `yaml:"network devices,omitempty"`
	Tags           map[string]string      `yaml:"tags,omitempty"`
	PrimaryNetDev  string                 `yaml:"primary network,omitempty" lopt:"primarynet" sopt:"p" comment:"Set the primary network interface"`
	Disks          map[string]*Disk       `yaml:"disks,omitempty"`
	FileSystems    map[string]*FileSystem `yaml:"filesystems,omitempty"`
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
Returns the id of the profile
*/
func (node *Profile) Id() string {
	return node.id
}

/*
Flattens out a Profile, which means if there are no explicit values in *IPMI
or *Kernel, these pointer will set to nil. This will remove something like
ipmi: {} from nodes.conf
*/
func (info *Profile) Flatten() {
	recursiveFlatten(info)
}
