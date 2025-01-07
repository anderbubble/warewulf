package api

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/url"
	"os"
	"path"

	"github.com/swaggest/usecase"
	api_overlay "github.com/warewulf/warewulf/internal/pkg/api/overlay"
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

func (overlayFile *OverlayFile) FullPath() string {
	return path.Join(overlay.GetOverlay(overlayFile.Overlay).Rootfs(), overlayFile.Path)
}

func (overlayFile *OverlayFile) Exists() bool {
	return overlay.GetOverlay(overlayFile.Overlay).Exists() && util.IsFile(overlayFile.FullPath())
}

func (overlayFile *OverlayFile) readContents() (string, error) {
	f, err := os.ReadFile(overlayFile.FullPath())
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

func createOverlay() usecase.Interactor {
	type createOverlayInput struct {
		Name string `path:"name"`
	}

	u := usecase.NewInteractor(func(ctx context.Context, input createOverlayInput, output *Overlay) error {
		err := overlay.GetSiteOverlay(input.Name).Create()
		if err != nil {
			return err
		}
		*output = *NewOverlay(input.Name)
		return nil
	})
	u.SetTitle("Create a new overlay")
	u.SetDescription("Create a new overlay")
	u.SetTags("Overlay")
	return u
}

func buildOverlay() usecase.Interactor {
	type buildOverlayInput struct {
		NodeNames    []string `json:"nodes" required:"true" description:"Node names for building overlays"`
		OverlayNames []string `json:"overlayNames" required:"true" description:"Specified overlay names"`
		OverlayDir   string   `json:"overlayDir" description:"Overlay dir"`
	}
	u := usecase.NewInteractor(func(ctx context.Context, input *buildOverlayInput, output *map[string]*Overlay) error {
		err := api_overlay.BuildOverlay(&api_overlay.BuildOverlayParameter{
			NodeNames:    input.NodeNames,
			OverlayDir:   input.OverlayDir,
			OverlayNames: input.OverlayNames,
		})
		if err != nil {
			return fmt.Errorf("failed to build overlays, err: %w", err)
		}

		m := make(map[string]*Overlay, len(input.OverlayNames))
		for _, name := range input.OverlayNames {
			m[name] = NewOverlay(name)
		}
		*output = m
		return nil
	})
	u.SetTitle("Build an overlay")
	u.SetDescription("Build an overlay based on given overlay name")
	u.SetTags("Overlay")
	return u
}

func deleteOverlay() usecase.Interactor {
	type deleteOverlayInput struct {
		OverlayName string `path:"name" description:"Overlay name to delete"`
		FilePath    string `path:"path" description:"File name to delete"`
		Parents     bool   `query:"parents" default:"true" description:"Whether to delete empty parent folders"`
		Force       bool   `query:"force" default:"false" description:"Whether do delete overlays forcely"`
	}

	u := usecase.NewInteractor(func(ctx context.Context, input *deleteOverlayInput, output *Overlay) error {
		if !overlay.GetOverlay(input.OverlayName).Exists() {
			return fmt.Errorf("overlay not found: %v", input.OverlayName)
		} else {
			*output = *NewOverlay(input.OverlayName)
		}

		return api_overlay.DeleteOverlay(&api_overlay.DeleteOverlayParameter{
			OverlayName: input.OverlayName,
			FilePath:    input.FilePath,
			Parents:     input.Parents,
			Force:       input.Force,
		})
	})
	u.SetTitle("Delete an overlay")
	u.SetDescription("Delete an overlay")
	u.SetTags("Overlay")
	return u
}

func renderOverlay() usecase.Interactor {
	type renderOverlayInput struct {
		OverlayName string `path:"name" description:"Overlay name to render"`
		FilePath    string `path:"path" description:"File path to render"`
		NodeName    string `query:"nodeName" description:"Node name to render"`
	}

	u := usecase.NewInteractor(func(ctx context.Context, input *renderOverlayInput, output *string) error {
		text, err := api_overlay.ShowOverlay(&api_overlay.ShowOverlayParameter{
			OverlayName: input.OverlayName,
			FilePath:    input.FilePath,
			NodeName:    input.NodeName,
			Quiet:       true,
		})
		if err != nil {
			return err
		}
		*output = text
		return nil
	})
	u.SetTitle("Render an overlay")
	u.SetDescription("Render an overlay")
	u.SetTags("Overlay")
	return u
}

func importOverlay() usecase.Interactor {
	type importOverlayInput struct {
		OverlayName string         `path:"name" description:"Overlay name to render"`
		Dest        string         `path:"path" description:"Destination path"`
		File        multipart.File `formData:"upload" description:"File to upload"`
	}

	u := usecase.NewInteractor(func(ctx context.Context, input *importOverlayInput, output *Overlay) error {
		tmpfile, err := os.CreateTemp(os.TempDir(), "warewulfd-file-uploaded-*")
		if err != nil {
			return fmt.Errorf("failed to initialize the temp file, err: %w", err)
		}
		if input.File == nil {
			return fmt.Errorf("no upload file")
		}

		_, err = io.Copy(tmpfile, input.File)
		if err != nil {
			return fmt.Errorf("failed to upload file, err: %w", err)
		}
		// close tmp file for opening in the next step
		_ = tmpfile.Close()
		_ = input.File.Close()

		err = api_overlay.ImportOverlay(&api_overlay.ImportOverlayParameter{
			OverlayName:     input.OverlayName,
			Source:          tmpfile.Name(),
			Dest:            input.Dest,
			CreateDirs:      true,
			NoOverlayUpdate: true,
		})
		if err != nil {
			return err
		}
		*output = *NewOverlay(input.OverlayName)
		return nil
	})
	u.SetTitle("Import a file to overlay")
	u.SetDescription("Import a file to overlay")
	u.SetTags("Overlay")
	return u
}
