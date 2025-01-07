package overlay

import (
	"errors"
	"fmt"
	"strings"
	"syscall"

	"github.com/warewulf/warewulf/internal/pkg/hostlist"
	"github.com/warewulf/warewulf/internal/pkg/node"
	"github.com/warewulf/warewulf/internal/pkg/overlay"
)

// we do not have a grpc protobuf definition for overlay related apis
// we will remove old grpc legacy support in the future, this overlay api
// is used for both cli and rest apis
type BuildOverlayParameter struct {
	NodeNames    []string `json:"nodeNames,omitempty" yaml:"nodeNames,omitempty"`
	OverlayDir   string   `json:"overlayDir,omitempty" yaml:"overlayDir,omitempty"`
	OverlayNames []string `json:"overlayNames,omitempty" yaml:"overlayNames,omitempty"`
}

func BuildOverlay(buildOverlayParam *BuildOverlayParameter) error {
	nodeDB, err := node.New()
	if err != nil {
		return fmt.Errorf("could not open node configuration: %s", err)
	}

	db, err := nodeDB.FindAllNodes()
	if err != nil {
		return fmt.Errorf("could not get node list: %s", err)
	}

	nodeNames := buildOverlayParam.NodeNames
	if len(nodeNames) > 0 {
		nodeNames = hostlist.Expand(nodeNames)
		db = node.FilterNodeListByName(db, nodeNames)

		if len(db) < len(nodeNames) {
			return errors.New("failed to find nodes")
		}
	}

	// NOTE: this is to keep backward compatible
	// passing -O a,b,c versus -O a -O b -O c, but will also accept -O a,b -O c
	OverlayNames := buildOverlayParam.OverlayNames
	overlayNames := []string{}
	for _, name := range OverlayNames {
		names := strings.Split(name, ",")
		overlayNames = append(overlayNames, names...)
	}
	OverlayNames = overlayNames

	if buildOverlayParam.OverlayDir != "" {
		if len(OverlayNames) == 0 {
			// TODO: should this behave the same as OverlayDir == "", and build default
			// set to overlays?
			return errors.New("must specify overlay(s) to build")
		}

		if len(nodeNames) > 0 {
			if len(db) != 1 {
				return errors.New("must specify one node to build overlay")
			}

			for _, node := range db {
				return overlay.BuildOverlayIndir(node, OverlayNames, buildOverlayParam.OverlayDir)
			}
		} else {
			return errors.New("must specify a node to build overlay")
		}

	}

	oldMask := syscall.Umask(0o07)
	defer syscall.Umask(oldMask)

	if len(OverlayNames) > 0 {
		err = overlay.BuildSpecificOverlays(db, OverlayNames)
	} else {
		err = overlay.BuildAllOverlays(db)
	}

	if err != nil {
		return fmt.Errorf("some overlays failed to be generated: %s", err)
	}
	return nil
}
