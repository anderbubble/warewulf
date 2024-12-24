package imprt

import (
	"fmt"

	"github.com/spf13/cobra"
	apicontainer "github.com/warewulf/warewulf/internal/pkg/api/container"
	"github.com/warewulf/warewulf/internal/pkg/api/routes/wwapiv1"
	"github.com/warewulf/warewulf/internal/pkg/warewulfd"
	"github.com/warewulf/warewulf/internal/pkg/wwlog"
)

func CobraRunE(cmd *cobra.Command, args []string) (err error) {
	// Shim in a name if none given.
	name := ""
	if len(args) == 2 {
		name = args[1]
	}

	cip := &wwapiv1.ContainerImportParameter{
		Source:      args[0],
		Name:        name,
		Force:       SetForce,
		Update:      SetUpdate,
		Build:       SetBuild,
		Default:     SetDefault,
		SyncUser:    SyncUser,
		OciNoHttps:  OciNoHttps,
		OciUsername: OciUsername,
		OciPassword: OciPassword,
		Platform:    Platform,
	}

	_, err = apicontainer.ContainerImport(cip)

	if SetDefault && err != nil {
		wwlog.Info("Set default profile to container: %s", cip.Name)
		err = warewulfd.DaemonStatus()
		if err != nil {
			// warewulfd is not running, skip
			return nil
		}

		err = warewulfd.DaemonReload()
		if err != nil {
			err = fmt.Errorf("failed to reload warewulf daemon: %w", err)
			return
		}
	}
	return
}
