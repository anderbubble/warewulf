package show

import (
	"github.com/spf13/cobra"
	"github.com/warewulf/warewulf/internal/pkg/api/overlay"
)

func CobraRunE(cmd *cobra.Command, args []string) error {
	overlayName := args[0]
	fileName := args[1]

	_, err := overlay.ShowOverlay(&overlay.ShowOverlayParameter{
		OverlayName: overlayName,
		FilePath:    fileName,
		NodeName:    NodeName,
		Quiet:       Quiet,
	})
	return err
}
