package list

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/warewulf/warewulf/internal/pkg/testenv"
	"github.com/warewulf/warewulf/internal/pkg/warewulfd"
	"github.com/warewulf/warewulf/internal/pkg/wwlog"
)

func Test_List_Args(t *testing.T) {
	tests := []struct {
		args   []string
		output string
		fail   bool
	}{
		{args: []string{""},
			output: `  CONTAINER NAME
  test
`,
			fail: false,
		},
		{args: []string{"-l"},
			output: `  CONTAINER NAME  NODES  KERNEL VERSION  CREATION TIME        MODIFICATION TIME    SIZE
  test            0                      02 Jan 00 03:04 UTC  01 Jan 70 00:00 UTC  0 B
`,
			fail: false,
		},
		{args: []string{"-c"},
			output: `  CONTAINER NAME  NODES  SIZE
  test            0      37 B
`,
			fail: false,
		},
	}

	warewulfd.SetNoDaemon()
	for _, tt := range tests {
		env := testenv.New(t)
		defer env.RemoveAll(t)
		env.WriteFile(t, "etc/warewulf/nodes.conf", tt.inDb)
		env.WriteFile(t, path.Join(testenv.WWChrootdir, "test/rootfs/bin/sh"), `This is a fake shell, no pearls here.`)
		// need to touch the files, so that the creation date of the container is constant,
		// modification date of `../chroots/containername` is used as creation date.
		// modification dates of directories change every time a file or subdir is added
		// so we have to make it constant *after* its creation.
		cmd := exec.Command("touch", "-d", "2000-01-02 03:04:05 UTC",
			env.GetPath(path.Join(testenv.WWChrootdir, "test/rootfs")),
			env.GetPath(path.Join(testenv.WWChrootdir, "test")))
		err := cmd.Run()
		assert.NoError(t, err)

		t.Logf("Running test: %s\n", tt.name)
		t.Run(strings.Join(tt.args, "_"), func(t *testing.T) {
			buf := new(bytes.Buffer)
			baseCmd := GetCommand()
			baseCmd.SetArgs(tt.args)
			baseCmd.SetOut(nil)
			baseCmd.SetErr(nil)
			wwlog.SetLogWriter(buf)
			err := baseCmd.Execute()
			if tt.fail {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Contains(t,
				strings.Join(strings.Fields(buf.String()), ""),
				strings.Join(strings.Fields(tt.stdout), ""))
		})
	}
}
