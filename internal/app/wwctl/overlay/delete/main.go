package delete

import (
	"github.com/spf13/cobra"
	"github.com/warewulf/warewulf/internal/pkg/api/overlay"
)

func CobraRunE(cmd *cobra.Command, args []string) error {
	var fileName string

	overlayName := args[0]

	if len(args) == 2 {
		fileName = args[1]
	}

	return overlay.DeleteOverlay(&overlay.DeleteOverlayParameter{
		OverlayName: overlayName,
		FilePath:    fileName,
		Parents:     Parents,
		Force:       Force,
	})
}
