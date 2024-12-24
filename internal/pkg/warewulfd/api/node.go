package api

import (
	"context"
	"fmt"

	"github.com/swaggest/usecase"
	"github.com/swaggest/usecase/status"
	"github.com/warewulf/warewulf/internal/pkg/node"
)

func getRawNodes() usecase.Interactor {
	u := usecase.NewInteractor(func(ctx context.Context, _ struct{}, output *map[string]*node.Node) error {
		if registry, err := node.New(); err != nil {
			return err
		} else {
			*output = registry.Nodes
			return nil
		}
	})
	u.SetTitle("Get raw nodes")
	u.SetDescription("Get all nodes, without merging in values from associated profiles.")
	u.SetTags("Node")
	return u
}

func getRawNodeByID() usecase.Interactor {
	type getNodeByIDInput struct {
		ID string `path:"id"`
	}

	u := usecase.NewInteractor(func(ctx context.Context, input getNodeByIDInput, output *node.Node) error {
		if registry, err := node.New(); err != nil {
			return err
		} else {
			if node_, err := registry.GetNodeOnly(input.ID); err != nil {
				return status.Wrap(fmt.Errorf("node not found: %v (%v)", input.ID, err), status.NotFound)
			} else {
				*output = node_
				return nil
			}
		}
	})
	u.SetTitle("Get a raw node")
	u.SetDescription("Get a node by its ID, without merging in values from associated profiles.")
	u.SetTags("Node")
	u.SetExpectedErrors(status.NotFound)
	return u
}

func putRawNodeByID() usecase.Interactor {
	type putNodeByIDInput struct {
		ID   string    `path:"id"`
		Node node.Node `json:"node"`
	}

	u := usecase.NewInteractor(func(ctx context.Context, input putNodeByIDInput, output *node.Node) error {
		if registry, err := node.New(); err != nil {
			return fmt.Errorf("error accessing node registry: %v", err)
		} else {
			if _, ok := registry.Nodes[input.ID]; !ok {
				_, _ = registry.AddNode(input.ID)
			}
			if err := registry.SetNode(input.ID, input.Node); err != nil {
				return fmt.Errorf("error setting node: %v (%v)", input.ID, err)
			} else {
				if node_, err := registry.GetNodeOnly(input.ID); err != nil {
					return fmt.Errorf("node not found after set: %v (%v)", input.ID, err)
				} else {
					*output = node_
					if err := registry.Persist(); err != nil {
						return fmt.Errorf("error persisting node registry: %v", err)
					}
					return nil
				}
			}
		}
	})
	u.SetTitle("Add or update a raw node")
	u.SetDescription("Add or update a raw node and get the resultant node without merging in values from associated profiles.")
	u.SetTags("Node")

	return u
}

func getNodes() usecase.Interactor {
	u := usecase.NewInteractor(func(ctx context.Context, _ struct{}, output *map[string]*node.Node) error {
		if registry, err := node.New(); err != nil {
			return err
		} else {
			nodeMap := make(map[string]*node.Node)
			if nodeList, err := registry.FindAllNodes(); err != nil {
				return err
			} else {
				for _, n := range nodeList {
					nodeMap[n.Id()] = &n
				}
				*output = nodeMap
				return nil
			}
		}
	})
	u.SetTitle("Get nodes")
	u.SetDescription("Get all nodes, including values from associated profiles.")
	u.SetTags("Node")
	return u
}

func getNodeByID() usecase.Interactor {
	type getNodeByIDInput struct {
		ID string `path:"id"`
	}

	u := usecase.NewInteractor(func(ctx context.Context, input getNodeByIDInput, output *node.Node) error {
		if registry, err := node.New(); err != nil {
			return err
		} else {
			if node_, err := registry.GetNode(input.ID); err != nil {
				return status.Wrap(fmt.Errorf("node not found: %v (%v)", input.ID, err), status.NotFound)
			} else {
				*output = node_
				return nil
			}
		}
	})
	u.SetTitle("Get a node")
	u.SetDescription("Get a node by its ID, including values from associated profiles.")
	u.SetTags("Node")
	u.SetExpectedErrors(status.NotFound)
	return u
}
