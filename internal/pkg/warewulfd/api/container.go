package api

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/swaggest/usecase"
	container_api "github.com/warewulf/warewulf/internal/pkg/api/container"
	"github.com/warewulf/warewulf/internal/pkg/api/routes/wwapiv1"
	"github.com/warewulf/warewulf/internal/pkg/container"
	"github.com/warewulf/warewulf/internal/pkg/kernel"
)

type Container struct {
	Kernels   []string `json:"kernels"`
	Size      int      `json:"size"`
	BuildTime int64    `json:"buildtime"`
	Writable  bool     `json:"writable"`
}

func NewContainer(name string) *Container {
	c := new(Container)
	c.Kernels = []string{}
	for _, k := range kernel.FindKernels(name) {
		c.Kernels = append(c.Kernels, k.Path)
	}
	c.Size = container.ImageSize(name)
	modTime := container.ImageModTime(name)
	if modTime.IsZero() {
		c.BuildTime = 0
	} else {
		c.BuildTime = modTime.Unix()
	}
	c.Writable = container.IsWriteAble(name)
	return c
}

func getContainers() usecase.Interactor {
	u := usecase.NewInteractor(func(ctx context.Context, _ struct{}, output *map[string]*Container) error {
		m := make(map[string]*Container)
		if names, err := container.ListSources(); err != nil {
			return err
		} else {
			for _, name := range names {
				m[name] = NewContainer(name)
			}
			*output = m
			return nil
		}
	})
	u.SetTitle("Get all containers")
	u.SetDescription("Get all defined containers")
	u.SetTags("Container")
	return u
}

func getContainerByName() usecase.Interactor {
	type getContainerByNameInput struct {
		Name string `path:"name"`
	}

	u := usecase.NewInteractor(func(ctx context.Context, input getContainerByNameInput, output *Container) error {
		if !container.ValidSource(input.Name) {
			return fmt.Errorf("container not found: %v", input.Name)
		} else {
			*output = *NewContainer(input.Name)
			return nil
		}
	})
	u.SetTitle("Get a container")
	u.SetDescription("Get a container by its name")
	u.SetTags("Container")
	return u
}

func importContainer() usecase.Interactor {
	type importContainerInput struct {
		Name     string `path:"name"`
		URI      string `json:"uri" required:"true" description:"OCI registry URI to import container definition from"`
		NoHttps  bool   `json:"nohttps" default:"false" description:"Use http, rather than https, to communicate with the registry, default:'false'"`
		User     string `json:"user" description:"Username for the registry, if needed"`
		Password string `json:"password" description:"Password for the registry, if needed"`
	}

	u := usecase.NewInteractor(func(ctx context.Context, input importContainerInput, output *Container) error {
		if !strings.HasPrefix(input.URI, "docker://") {
			return errors.New("uri only supports docker:// prefix for now")
		}

		if !container.ValidName(input.Name) {
			return fmt.Errorf("VNFS name contains illegal characters: %s", input.Name)
		}

		sctx, err := container_api.GetSystemContext(input.NoHttps, input.User, input.Password, "")
		if err != nil {
			return err
		}

		err = container.ImportDocker(input.URI, input.Name, sctx)
		if err != nil {
			return err
		}

		*output = *NewContainer(input.Name)
		return nil
	})
	u.SetTitle("Import a container")
	u.SetDescription("Import a container from an OCI registry")
	u.SetTags("Container")

	return u
}

func deleteContainer() usecase.Interactor {
	type deleteContainerInput struct {
		Name string `path:"name"`
	}

	u := usecase.NewInteractor(func(ctx context.Context, input deleteContainerInput, output *Container) error {
		if !container.ValidSource(input.Name) {
			return fmt.Errorf("container not found: %v", input.Name)
		}

		*output = *NewContainer(input.Name)
		cdp := &wwapiv1.ContainerDeleteParameter{
			ContainerNames: []string{input.Name},
		}

		err := container_api.ContainerDelete(cdp)
		return err
	})
	u.SetTitle("Delete a container")
	u.SetDescription("Delete an existing container")
	u.SetTags("Container")

	return u
}

func renameContainer() usecase.Interactor {
	type renameContainerInput struct {
		Name   string `path:"name"`
		Target string `path:"target"`
		Build  bool   `query:"build" default:"true" description:"Build the container image after renaming, default:'true'"`
	}

	u := usecase.NewInteractor(func(ctx context.Context, input renameContainerInput, output *Container) error {
		crp := &wwapiv1.ContainerRenameParameter{
			ContainerName: input.Name,
			TargetName:    input.Target,
			Build:         input.Build,
		}

		err := container_api.ContainerRename(crp)
		if err != nil {
			return err
		}

		*output = *NewContainer(input.Target)
		return nil
	})
	u.SetTitle("Rename a container")
	u.SetDescription("Rename an existing container with a new name")
	u.SetTags("Container")

	return u
}

func buildContainer() usecase.Interactor {
	type buildContainerInput struct {
		Name  string `path:"name"`
		Force bool   `query:"force" default:"false" description:"Build the container image even if it appears unnecessary, default:'false'"`
	}

	u := usecase.NewInteractor(func(ctx context.Context, input buildContainerInput, output *Container) error {
		cbp := &wwapiv1.ContainerBuildParameter{
			ContainerNames: []string{input.Name},
			Force:          input.Force,
		}

		err := container_api.ContainerBuild(cbp)
		if err != nil {
			return err
		}

		*output = *NewContainer(input.Name)
		return nil
	})
	u.SetTitle("Build a container")
	u.SetDescription("Build a container image")
	u.SetTags("Container")

	return u
}
