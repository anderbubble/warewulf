package imprt

import (
	"github.com/spf13/cobra"
	"github.com/warewulf/warewulf/internal/pkg/api/overlay"
)

func CobraRunE(cmd *cobra.Command, args []string) (err error) {
	var dest string

	overlayName := args[0]
	source := args[1]

	if len(args) == 3 {
		dest = args[2]
	} else {
		dest = source
	}

	return overlay.ImportOverlay(&overlay.ImportOverlayParameter{
		OverlayName:     overlayName,
		Source:          source,
		Dest:            dest,
		CreateDirs:      CreateDirs,
		NoOverlayUpdate: NoOverlayUpdate,
	})
}
