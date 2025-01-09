package api

import (
	"context"
	"fmt"
	"runtime"
	"sort"

	"dario.cat/mergo"
	"github.com/swaggest/usecase"
	"github.com/swaggest/usecase/status"
	"github.com/warewulf/warewulf/internal/pkg/node"
	"github.com/warewulf/warewulf/internal/pkg/overlay"
	"github.com/warewulf/warewulf/internal/pkg/warewulfd/daemon"
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
				for i := range nodeList {
					nodeMap[nodeList[i].Id()] = &nodeList[i]
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

func addNode() usecase.Interactor {
	type addNodeInput struct {
		ID   string    `path:"id" required:"true" description:"Node name"`
		Node node.Node `json:"node" required:"true" description:"Node to add in json format"`
	}

	u := usecase.NewInteractor(func(ctx context.Context, input addNodeInput, output *node.Node) error {
		registry, regErr := node.New()
		if regErr != nil {
			return regErr
		}

		registry.Nodes[input.ID] = &input.Node
		persistErr := registry.Persist()
		if persistErr != nil {
			return persistErr
		}

		*output = *(registry.Nodes[input.ID])

		return daemon.DaemonReload()
	})
	u.SetTitle("Add a node")
	u.SetDescription("Add a new node")
	u.SetTags("Node")

	return u
}

func deleteNode() usecase.Interactor {
	type deleteNodeInput struct {
		ID string `path:"id" description:"Node id needs to be deleted"`
	}

	u := usecase.NewInteractor(func(ctx context.Context, input deleteNodeInput, output *node.Node) error {
		registry, regErr := node.New()
		if regErr != nil {
			return regErr
		}

		node, ok := registry.Nodes[input.ID]
		if !ok {
			return fmt.Errorf("node '%s' does not exist", input.ID)
		}

		delErr := registry.DelNode(input.ID)
		if delErr != nil {
			return delErr
		}

		*output = *node
		return nil
	})
	u.SetTitle("Delete an existing node")
	u.SetDescription("Delete an existing node")
	u.SetTags("Node")

	return u
}

func updateNode() usecase.Interactor {
	type updateNodeInput struct {
		Name     string    `path:"id" description:"Node id needs to be updated"`
		NodeConf node.Node `json:"node" required:"true" description:"Node structure"`
	}

	u := usecase.NewInteractor(func(ctx context.Context, input updateNodeInput, output *node.Node) error {
		nodeDB, err := node.New()
		if err != nil {
			return fmt.Errorf("failed to initialize nodeDB, err: %w", err)
		}
		nodePtr, err := nodeDB.GetNodeOnlyPtr(input.Name)
		if err != nil {
			return fmt.Errorf("failed to retrieve node by its id, err: %w", err)
		}
		err = mergo.MergeWithOverwrite(nodePtr, &input.NodeConf)
		if err != nil {
			return err
		}

		err = nodeDB.Persist()
		if err != nil {
			return err
		}

		*output = *nodePtr
		return nil
	})
	u.SetTitle("Update an existing node")
	u.SetDescription("Update an existing node")
	u.SetTags("Node")

	return u
}

func buildAllOverlays() usecase.Interactor {
	u := usecase.NewInteractor(func(ctx context.Context, _ struct{}, output *[]string) error {
		nodeDB, err := node.New()
		if err != nil {
			return fmt.Errorf("could not open node configuration: %w", err)
		}

		allNodes, err := nodeDB.FindAllNodes()
		if err != nil {
			return fmt.Errorf("could not get node list: %w", err)
		}

		ret := make([]string, len(allNodes))
		for i := range allNodes {
			ret[i] = allNodes[i].Id()
		}
		sort.Strings(ret)

		err = overlay.BuildAllOverlays(allNodes, allNodes, runtime.NumCPU())
		if err != nil {
			return err
		}

		*output = ret
		return nil
	})
	return u
}

func buildOverlays() usecase.Interactor {
	type buildOverlayInput struct {
		Name string `path:"id" description:"Node id to build its all overlays"`
	}
	u := usecase.NewInteractor(func(ctx context.Context, input *buildOverlayInput, output *string) error {
		nodeDB, err := node.New()
		if err != nil {
			return fmt.Errorf("could not open node configuration: %w", err)
		}

		allNodes, err := nodeDB.FindAllNodes()
		if err != nil {
			return fmt.Errorf("could not get node list: %w", err)
		}

		targetNode, err := nodeDB.GetNode(input.Name)
		if err != nil {
			return fmt.Errorf("failed to get node with id: %s", input.Name)
		}

		err = overlay.BuildAllOverlays([]node.Node{targetNode}, allNodes, runtime.NumCPU())
		if err != nil {
			return err
		}

		*output = input.Name
		return nil
	})
	return u
}
