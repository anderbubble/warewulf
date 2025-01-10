package api

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path"

	"github.com/swaggest/usecase"
	"github.com/swaggest/usecase/status"
	"github.com/warewulf/warewulf/internal/pkg/node"
	"github.com/warewulf/warewulf/internal/pkg/overlay"
	"github.com/warewulf/warewulf/internal/pkg/util"
)

type Overlay struct {
	Files []string `json:"files"`
}

func NewOverlay(name string) *Overlay {
	o := new(Overlay)
	o.Files = []string{}
	if files, err := overlay.OverlayGetFiles(name); err == nil {
		o.Files = files
	}
	return o
}

func getOverlays() usecase.Interactor {
	u := usecase.NewInteractor(func(ctx context.Context, _ struct{}, output *map[string]*Overlay) error {
		m := make(map[string]*Overlay)
		if names, err := overlay.FindOverlays(); err != nil {
			return err
		} else {
			for _, name := range names {
				m[name] = NewOverlay(name)
			}
			*output = m
			return nil
		}
	})
	u.SetTitle("Get overlays")
	u.SetDescription("Get all overlays.")
	u.SetTags("Overlay")
	return u
}

func getOverlayByName() usecase.Interactor {
	type getOverlayByNameInput struct {
		Name string `path:"name"`
	}

	u := usecase.NewInteractor(func(ctx context.Context, input getOverlayByNameInput, output *Overlay) error {
		if !overlay.GetOverlay(input.Name).Exists() {
			return fmt.Errorf("overlay not found: %v", input.Name)
		} else {
			*output = *NewOverlay(input.Name)
			return nil
		}
	})
	u.SetTitle("Get an overlay")
	u.SetDescription("Get an overlay by its name.")
	u.SetTags("Overlay")
	return u
}

type OverlayFile struct {
	Overlay  string `json:"overlay"`
	Path     string `json:"path"`
	Contents string `json:"contents"`
	rendered bool
}

func (this *OverlayFile) FullPath() string {
	return path.Join(overlay.GetOverlay(this.Overlay).Rootfs(), this.Path)
}

func (this *OverlayFile) Exists() bool {
	return overlay.GetOverlay(this.Overlay).Exists() && util.IsFile(this.FullPath())
}

func (this *OverlayFile) readContents() (string, error) {
	f, err := os.ReadFile(this.FullPath())
	return string(f), err
}

func (this *OverlayFile) renderContents(nodeName string) (string, error) {
	if !(path.Ext(this.Path) == ".ww") {
		return "", fmt.Errorf("'%s' does not end with '.ww'", this.Path)
	}

	if this.rendered {
		return "", fmt.Errorf("already rendered")
	}

	registry, regErr := node.New()
	if regErr != nil {
		return "", regErr
	}

	renderNode, nodeErr := registry.GetNode(nodeName)
	if nodeErr != nil {
		return "", nodeErr
	}

	allNodes, allNodesErr := registry.FindAllNodes()
	if allNodesErr != nil {
		return "", allNodesErr
	}

	tstruct, structErr := overlay.InitStruct(this.Overlay, renderNode, allNodes)
	if structErr != nil {
		return "", structErr
	}
	tstruct.BuildSource = this.Path

	buffer, _, _, renderErr := overlay.RenderTemplateFile(this.FullPath(), tstruct)
	if renderErr != nil {
		return "", renderErr
	}

	return buffer.String(), nil
}

func NewOverlayFile(name string, path string, renderNodeName string) (*OverlayFile, error) {
	this := new(OverlayFile)
	this.Overlay = name
	this.Path = path
	if renderNodeName == "" {
		if contents, err := this.readContents(); err != nil {
			return this, err
		} else {
			this.Contents = contents
		}
	} else {
		if contents, err := this.renderContents(renderNodeName); err != nil {
			return this, err
		} else {
			this.Contents = contents
		}
	}
	return this, nil
}

func getOverlayFile() usecase.Interactor {
	type getOverlayByNameInput struct {
		Name string `path:"name"`
		Path string `query:"path"`
		Node string `query:"render"`
	}

	u := usecase.NewInteractor(func(ctx context.Context, input getOverlayByNameInput, output *OverlayFile) error {
		if input.Path == "" {
			return status.Wrap(fmt.Errorf("must specify a path"), status.InvalidArgument)
		}

		relPath, parseErr := url.QueryUnescape(input.Path)
		if parseErr != nil {
			return fmt.Errorf("failed to decode path: %v: %w", input.Path, parseErr)
		}

		overlayFile, err := NewOverlayFile(input.Name, relPath, input.Node)
		if err != nil {
			return fmt.Errorf("unable to read overlay file %v: %v: %w", input.Name, relPath, err)
		}

		*output = *overlayFile
		return nil
	})
	u.SetTitle("Get an overlay file")
	u.SetDescription("Get an overlay file by its name and path, optionally rendered for a given node.")
	u.SetTags("Overlay")
	return u
}

func createOverlay() usecase.Interactor {
	type createOverlayInput struct {
		Name string `path:"name"`
	}

	u := usecase.NewInteractor(func(ctx context.Context, input createOverlayInput, output *Overlay) error {
		newOverlay := overlay.GetSiteOverlay(input.Name)
		if err := newOverlay.Create(); err != nil {
			return err
		}
		*output = *NewOverlay(newOverlay.Name())
		return nil
	})
	u.SetTitle("Create an overlay")
	u.SetDescription("Create an overlay.")
	u.SetTags("Overlay")
	return u
}
