package api

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path"

	"github.com/swaggest/usecase"
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

func NewOverlayFile(name string, path string) (*OverlayFile, error) {
	f := new(OverlayFile)
	f.Overlay = name
	f.Path = path
	if contents, err := f.readContents(); err != nil {
		return f, err
	} else {
		f.Contents = contents
		return f, nil
	}
}

func getOverlayFile() usecase.Interactor {
	type getOverlayByNameInput struct {
		Name string `path:"name"`
		Path string `path:"path"`
	}

	u := usecase.NewInteractor(func(ctx context.Context, input getOverlayByNameInput, output *OverlayFile) error {
		if relPath, err := url.QueryUnescape(input.Path); err != nil {
			return fmt.Errorf("failed to decode path: %v (%v)", input.Path, err)
		} else {
			if overlayFile, err := NewOverlayFile(input.Name, relPath); err != nil {
				return fmt.Errorf("unable to read overlay file %v:%v", input.Name, relPath)
			} else {
				*output = *overlayFile
				return nil
			}
		}
	})
	u.SetTitle("Get an overlay file")
	u.SetDescription("Get an overlay file by its name and path.")
	u.SetTags("Overlay")
	return u
}
