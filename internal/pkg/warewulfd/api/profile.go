package api

import (
	"context"
	"fmt"

	"dario.cat/mergo"
	"github.com/swaggest/usecase"
	"github.com/swaggest/usecase/status"
	apiprofile "github.com/warewulf/warewulf/internal/pkg/api/profile"
	"github.com/warewulf/warewulf/internal/pkg/api/routes/wwapiv1"
	"github.com/warewulf/warewulf/internal/pkg/node"
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
		Profile   node.Profile     `json:"profile" required:"true" description:"Profile to add in json format"`
		NodeAdd   node.NodeConfAdd `json:"nodeAddConf" description:"Node add structure"`
		NodeNames []string         `json:"names" required:"true" description:"Node names for adding the profile"`
	}

	u := usecase.NewInteractor(func(ctx context.Context, input addProfileInput, output *node.Profile) error {
		add, err := apiprofile.ConvertAddProfile(&apiprofile.AddProfileParameter{
			ProfileConf: input.Profile,
			NodeAdd:     input.NodeAdd,
			NodeNames:   input.NodeNames,
		})
		if err != nil {
			return fmt.Errorf("failed to convert cli inputs to profiles add operation, err: %w", err)
		}

		err = apiprofile.ProfileAdd(add)
		if err != nil {
			return err
		}
		*output = input.Profile
		return nil
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
		return nil
	})
	u.SetTitle("Update an existing profile")
	u.SetDescription("Update an existing profile")
	u.SetTags("Profile")

	return u
}

func deleteProfile() usecase.Interactor {
	type deleteProfileInput struct {
		Name string `path:"id" description:"Profile id needs to be deleted"`
	}

	u := usecase.NewInteractor(func(ctx context.Context, input deleteProfileInput, output *string) error {
		ndp := &wwapiv1.NodeDeleteParameter{
			NodeNames: []string{input.Name},
			Force:     true,
		}

		err := apiprofile.ProfileDelete(ndp)
		if err != nil {
			return err
		}
		*output = input.Name
		return nil
	})
	u.SetTitle("Delete an existing profile")
	u.SetDescription("Delete an existing profile")
	u.SetTags("Profile")

	return u
}
