package build

import (
	"github.com/spf13/cobra"
	"github.com/warewulf/warewulf/internal/pkg/api/overlay"
)

func CobraRunE(cmd *cobra.Command, args []string) error {
	return overlay.BuildOverlay(&overlay.BuildOverlayParameter{
		OverlayDir:   OverlayDir,
		OverlayNames: OverlayNames,
		NodeNames:    args,
	})
}
