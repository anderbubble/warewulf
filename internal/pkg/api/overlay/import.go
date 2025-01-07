package overlay

import (
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/warewulf/warewulf/internal/pkg/node"
	"github.com/warewulf/warewulf/internal/pkg/overlay"
	"github.com/warewulf/warewulf/internal/pkg/util"
	"github.com/warewulf/warewulf/internal/pkg/wwlog"
)

type ImportOverlayParameter struct {
	OverlayName     string `json:"overlayName,omitempty" yaml:"overlayName,omitempty"`
	Source          string `json:"source,omitempty" yaml:"source,omitempty"`
	Dest            string `json:"dest,omitempty" yaml:"dest,omitempty"`
	CreateDirs      bool   `json:"createDirs,omitempty" yaml:"createDirs,omitempty"`
	NoOverlayUpdate bool   `json:"noOverlayUpdate,omitempty" yaml:"noOverlayUpdate,omitempty"`
}

func ImportOverlay(importParam *ImportOverlayParameter) (err error) {
	source := importParam.Source
	overlayName := importParam.OverlayName
	dest := importParam.Dest

	wwlog.Verbose("Copying '%s' into overlay '%s:%s'", source, overlayName, dest)
	overlay_ := overlay.GetOverlay(overlayName)
	if !overlay_.IsSiteOverlay() {
		overlay_, err = overlay_.CloneSiteOverlay()
		if err != nil {
			return err
		}
	}
	if !overlay_.Exists() {
		return fmt.Errorf("overlay does not exist: %s", overlayName)
	}

	if util.IsDir(overlay_.File(dest)) {
		dest = path.Join(dest, path.Base(source))
	}

	if util.IsFile(overlay_.File(dest)) {
		return fmt.Errorf("a file with that name already exists in the overlay: %s", overlayName)
	}

	if importParam.CreateDirs {
		parent := filepath.Dir(overlay_.File(dest))
		if _, err = os.Stat(parent); os.IsNotExist(err) {
			wwlog.Debug("Create dir: %s", parent)
			srcInfo, err := os.Stat(source)
			if err != nil {
				return fmt.Errorf("could not retrieve the stat for file: %s", err)
			}
			err = os.MkdirAll(parent, srcInfo.Mode())
			if err != nil {
				return fmt.Errorf("could not create parent dif: %s: %v", parent, err)
			}
		}
	}

	err = util.CopyFile(source, overlay_.File(dest))
	if err != nil {
		return fmt.Errorf("could not copy file into overlay: %w", err)
	}

	if !importParam.NoOverlayUpdate {
		n, err := node.New()
		if err != nil {
			return fmt.Errorf("could not open node configuration: %s", err)
		}

		nodes, err := n.FindAllNodes()
		if err != nil {
			return fmt.Errorf("could not get node list: %s", err)
		}

		var updateNodes []node.Node

		for _, node := range nodes {
			if util.InSlice(node.SystemOverlay, overlayName) {
				updateNodes = append(updateNodes, node)
			} else if util.InSlice(node.RuntimeOverlay, overlayName) {
				updateNodes = append(updateNodes, node)
			}
		}

		return overlay.BuildSpecificOverlays(updateNodes, []string{overlayName})
	}

	return nil
}
