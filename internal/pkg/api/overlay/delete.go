package overlay

import (
	"fmt"
	"os"
	"path"

	"github.com/warewulf/warewulf/internal/pkg/overlay"
	"github.com/warewulf/warewulf/internal/pkg/util"
	"github.com/warewulf/warewulf/internal/pkg/wwlog"
)

type DeleteOverlayParameter struct {
	OverlayName string `json:"overlayName,omitempty" yaml:"overlayName,omitempty"`
	FilePath    string `json:"filePath,omitempty" yaml:"filePath,omitempty"`
	Parents     bool   `json:"parents,omitempty" yaml:"parents,omitempty"`
	Force       bool   `json:"force,omitempty" yaml:"force,omitempty"`
}

func DeleteOverlay(deleteParam *DeleteOverlayParameter) error {
	overlayName := deleteParam.OverlayName
	filePath := deleteParam.FilePath

	overlay_ := overlay.GetOverlay(overlayName)
	if overlay_.IsDistributionOverlay() {
		return fmt.Errorf("distribution overlay can't deleted")
	}
	if !overlay_.Exists() {
		return fmt.Errorf("overlay does not exist: %s", overlayName)
	}

	if filePath == "" {
		if deleteParam.Force {
			err := os.RemoveAll(overlay_.Path())
			if err != nil {
				return fmt.Errorf("failed deleting overlay: %w", err)
			}
		} else {
			err := os.Remove(overlay_.Path())
			if err != nil {
				return fmt.Errorf("failed deleting overlay: %w", err)
			}
		}
	} else {
		removePath := overlay_.File(filePath)

		if !(util.IsDir(removePath) || util.IsFile(removePath)) {
			return fmt.Errorf("path to remove doesn't exist in overlay: %s", removePath)
		}

		if deleteParam.Force {
			err := os.RemoveAll(removePath)
			if err != nil {
				return fmt.Errorf("failed deleting file from overlay: %s:%s", overlayName, removePath)
			}
		} else {
			err := os.Remove(removePath)
			if err != nil {
				return fmt.Errorf("failed deleting overlay: %s:%s", overlayName, removePath)
			}
		}

		if deleteParam.Parents {
			// Cleanup any empty directories left behind...
			i := path.Dir(removePath)
			for i != overlay_.Rootfs() {
				wwlog.Debug("Evaluating directory to remove: %s", i)
				err := os.Remove(i)
				if err != nil {
					break
				}

				wwlog.Verbose("Removed empty directory: %s", i)
				i = path.Dir(i)
			}
		}
	}

	return nil
}
