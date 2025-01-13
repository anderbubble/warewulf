package api

import (
	"github.com/swaggest/openapi-go/openapi3"
	"github.com/swaggest/rest/web"
	swgui "github.com/swaggest/swgui/v5emb"

	"github.com/warewulf/warewulf/internal/pkg/version"
)

func Handler() *web.Service {
	api := web.NewService(openapi3.NewReflector())

	api.OpenAPISchema().SetTitle("Warewulf v4 API")
	api.OpenAPISchema().SetDescription("This service provides an API to a Warewulf v4 server.")
	api.OpenAPISchema().SetVersion(version.GetVersion())

	api.Get("/api/raw-nodes", getRawNodes())
	api.Get("/api/raw-nodes/{id}", getRawNodeByID())
	api.Put("/api/raw-nodes/{id}", putRawNodeByID())

	// node related rest apis
	api.Get("/api/nodes", getNodes())
	api.Get("/api/nodes/{id}", getNodeByID())
	api.Put("/api/nodes/{id}", addNode())
	api.Delete("/api/nodes/{id}", deleteNode())
	api.Patch("/api/nodes/{id}", updateNode())
	api.Post("/api/nodes/overlays/build", buildAllOverlays())
	api.Post("/api/nodes/{id}/overlays/build", buildOverlays())

	// profile related rest apis
	api.Get("/api/profiles", getProfiles())
	api.Get("/api/profiles/{id}", getProfileByID())
	api.Put("/api/profiles/{id}", addProfile())
	api.Patch("/api/profiles/{id}", updateProfile())
	api.Delete("/api/profiles/{id}", deleteProfile())

	// container related rest apis
	api.Get("/api/containers", getContainers())
	api.Get("/api/containers/{name}", getContainerByName())
	api.Post("/api/containers/{name}/import", importContainer())
	api.Delete("/api/containers/{name}", deleteContainer())
	api.Post("/api/containers/{name}/rename/{target}", renameContainer())
	api.Post("/api/containers/{name}/build", buildContainer())

	api.Get("/api/overlays", getOverlays())
	api.Get("/api/overlays/{name}", getOverlayByName())
	api.Get("/api/overlays/{name}/file", getOverlayFile())
	api.Put("/api/overlays/{name}", createOverlay())

	api.Docs("/api/docs", swgui.New)

	return api
}
