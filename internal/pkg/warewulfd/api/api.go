package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/swaggest/openapi-go/openapi3"
	"github.com/swaggest/rest/nethttp"
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

	// container related rest apis (with authentication)
	api.Route("/api/containers", func(r chi.Router) {
		// require "admin" role group
		r.Group(func(r chi.Router) {
			r.Use(AuthMiddleware, ACLMiddleware("admin"))
			r.Method(http.MethodDelete, "/{name}", nethttp.NewHandler(deleteContainer()))
		})
		// requrie "user" role group
		r.Group(func(r chi.Router) {
			r.Use(AuthMiddleware, ACLMiddleware("user"))
			r.Method(http.MethodGet, "/", nethttp.NewHandler(getContainers()))
			r.Method(http.MethodGet, "/{name}", nethttp.NewHandler(getContainerByName()))
			r.Method(http.MethodPost, "/{name}/import", nethttp.NewHandler(importContainer()))
			r.Method(http.MethodPost, "/{name}/rename/{target}", nethttp.NewHandler(renameContainer()))
			r.Method(http.MethodPost, "/{name}/build", nethttp.NewHandler(buildContainer()))
		})
	})

	api.Get("/api/overlays", getOverlays())
	api.Get("/api/overlays/{name}", getOverlayByName())
	api.Get("/api/overlays/{name}/file", getOverlayFile())
	api.Put("/api/overlays/{name}", createOverlay())
	api.Delete("/api/overlays/{name}", deleteOverlay())

	api.Docs("/api/docs", swgui.New)

	return api
}
