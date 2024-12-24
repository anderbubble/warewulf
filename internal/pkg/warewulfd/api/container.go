package api

import (
	"context"
	"fmt"

	"github.com/swaggest/usecase"
	"github.com/warewulf/warewulf/internal/pkg/container"
	"github.com/warewulf/warewulf/internal/pkg/kernel"
)

type Container struct {
	Kernels  []string `json:"kernels"`
	Size     int      `json:"size"`
	Writable bool     `json:"writable"`
}

func NewContainer(name string) *Container {
	c := new(Container)
	c.Kernels = []string{}
	for _, k := range kernel.FindKernels(name) {
		c.Kernels = append(c.Kernels, k.Path)
	}
	c.Size = container.ImageSize(name)
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
	u.SetTitle("Get container images")
	u.SetDescription("Get all container images.")
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
	u.SetDescription("Get a container by its name.")
	u.SetTags("Container")
	return u
}
