package api

import (
	"context"
	"fmt"

	"dario.cat/mergo"
	"github.com/swaggest/usecase"
	"github.com/swaggest/usecase/status"
	"github.com/warewulf/warewulf/internal/pkg/node"
	"github.com/warewulf/warewulf/internal/pkg/util"
	"github.com/warewulf/warewulf/internal/pkg/warewulfd/daemon"
)

func getProfiles() usecase.Interactor {
	u := usecase.NewInteractor(func(ctx context.Context, _ struct{}, output *map[string]*node.Profile) error {
		if registry, err := node.New(); err != nil {
			return err
		} else {
			*output = registry.NodeProfiles
			return nil
		}
	})
	u.SetTitle("Get node profiles")
	u.SetDescription("Get all node profiles.")
	u.SetTags("Profile")
	return u
}

func getProfileByID() usecase.Interactor {
	type getProfileByIDInput struct {
		ID string `path:"id"`
	}

	u := usecase.NewInteractor(func(ctx context.Context, input getProfileByIDInput, output *node.Profile) error {
		if registry, err := node.New(); err != nil {
			return err
		} else {
			if profile, err := registry.GetProfile(input.ID); err != nil {
				return status.Wrap(fmt.Errorf("profile not found: %v (%v)", input.ID, err), status.NotFound)
			} else {
				*output = profile
				return nil
			}
		}
	})
	u.SetTitle("Get a node profile")
	u.SetDescription("Get a node profile by its ID.")
	u.SetTags("Profile")
	u.SetExpectedErrors(status.NotFound)
	return u
}

func addProfile() usecase.Interactor {
	type addProfileInput struct {
		ID      string       `path:"id"`
		Profile node.Profile `json:"profile" required:"true" description:"Profile to add in json format"`
	}

	u := usecase.NewInteractor(func(ctx context.Context, input addProfileInput, output *node.Profile) error {
		registry, regErr := node.New()
		if regErr != nil {
			return regErr
		}

		registry.NodeProfiles[input.ID] = &input.Profile
		persistErr := registry.Persist()
		if persistErr != nil {
			return persistErr
		}

		*output = *(registry.NodeProfiles[input.ID])

		return daemon.DaemonReload()
	})
	u.SetTitle("Add a profile")
	u.SetDescription("Add a new profile for nodes")
	u.SetTags("Profile")

	return u
}

func updateProfile() usecase.Interactor {
	type updateProfileInput struct {
		Name        string       `path:"id" description:"Profile id needs to be updated"`
		ProfileConf node.Profile `json:"profile" required:"true" description:"Profile structure"`
	}

	u := usecase.NewInteractor(func(ctx context.Context, input updateProfileInput, output *node.Profile) error {
		nodeDB, err := node.New()
		if err != nil {
			return fmt.Errorf("failed to initialize nodeDB, err: %w", err)
		}
		profilePtr, err := nodeDB.GetProfilePtr(input.Name)
		if err != nil {
			return fmt.Errorf("failed to retrieve profile by its id, err: %w", err)
		}
		err = mergo.MergeWithOverwrite(profilePtr, &input.ProfileConf)
		if err != nil {
			return err
		}

		err = nodeDB.Persist()
		if err != nil {
			return err
		}

		*output = *profilePtr
		return daemon.DaemonReload()
	})
	u.SetTitle("Update an existing profile")
	u.SetDescription("Update an existing profile")
	u.SetTags("Profile")

	return u
}

func deleteProfile() usecase.Interactor {
	type deleteProfileInput struct {
		ID string `path:"id" description:"Profile id needs to be deleted"`
	}

	u := usecase.NewInteractor(func(ctx context.Context, input deleteProfileInput, output *node.Profile) error {
		registry, regErr := node.New()
		if regErr != nil {
			return regErr
		}

		profile, ok := registry.NodeProfiles[input.ID]
		if !ok {
			return fmt.Errorf("profile '%s' does not exist", input.ID)
		}

		nodesCount := 0
		for _, n := range registry.Nodes {
			if util.InSlice(n.Profiles, input.ID) {
				nodesCount++
			}
		}

		profilesCount := 0
		for _, p := range registry.NodeProfiles {
			if util.InSlice(p.Profiles, input.ID) {
				profilesCount++
			}
		}

		if nodesCount > 0 || profilesCount > 0 {
			return fmt.Errorf("profile '%s' is in use by %v nodes and %v profiles", input.ID, nodesCount, profilesCount)
		}

		delErr := registry.DelProfile(input.ID)
		if delErr != nil {
			return delErr
		}

		*output = *profile
		return daemon.DaemonReload()
	})
	u.SetTitle("Delete an existing profile")
	u.SetDescription("Delete an existing profile")
	u.SetTags("Profile")

	return u
}
