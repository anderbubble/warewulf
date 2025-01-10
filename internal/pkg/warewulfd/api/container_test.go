package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/warewulf/warewulf/internal/pkg/config"
	"github.com/warewulf/warewulf/internal/pkg/testenv"
)

func TestContainerAPI(t *testing.T) {
	env := testenv.New(t)
	defer env.RemoveAll()

	authData := `users:
  - name: admin
    pass: admin
    role: admin
  - name: user
    pass: user
    role: user`
	authentication := config.GetAuthentication()
	err := authentication.ParseFromRaw([]byte(authData))
	assert.NoError(t, err)

	t.Run("test all containers related apis", func(t *testing.T) {
		// prepareration
		srv := httptest.NewServer(Handler())
		defer srv.Close()
		env.WriteFile(path.Join(testenv.WWChrootdir, "test-container/rootfs/file"), `test`)

		// test no authentication
		req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/containers", nil)
		assert.NoError(t, err)

		// send request
		resp, err := http.DefaultTransport.RoundTrip(req)
		assert.NoError(t, err)

		// validate the resp
		body, err := io.ReadAll(resp.Body)
		assert.Equal(t, resp.StatusCode, http.StatusUnauthorized)
		assert.NoError(t, resp.Body.Close())
		assert.NoError(t, err)
		assert.Contains(t, string(body), "Unauthorized")

		// test get all containers
		req, err = http.NewRequest(http.MethodGet, srv.URL+"/api/containers", nil)
		assert.NoError(t, err)
		req.SetBasicAuth("user", "user")

		// send request
		resp, err = http.DefaultTransport.RoundTrip(req)
		assert.NoError(t, err)

		// validate the resp
		body, err = io.ReadAll(resp.Body)
		assert.NoError(t, resp.Body.Close())
		assert.NoError(t, err)
		assert.Contains(t, string(body), "test-container")

		// test get single container
		req, err = http.NewRequest(http.MethodGet, srv.URL+"/api/containers/test-container", nil)
		assert.NoError(t, err)
		req.SetBasicAuth("user", "user")

		// send request
		resp, err = http.DefaultTransport.RoundTrip(req)
		assert.NoError(t, err)

		// validate the resp
		body, err = io.ReadAll(resp.Body)
		assert.NoError(t, resp.Body.Close())
		assert.NoError(t, err)
		assert.NotEmpty(t, string(body))

		// test build container
		req, err = http.NewRequest(http.MethodPost, srv.URL+"/api/containers/test-container/build?force=true&default=true", nil)
		assert.NoError(t, err)
		req.SetBasicAuth("user", "user")

		// send request
		resp, err = http.DefaultTransport.RoundTrip(req)
		assert.NoError(t, err)

		// validate the resp
		body, err = io.ReadAll(resp.Body)
		assert.NoError(t, resp.Body.Close())
		assert.NoError(t, err)
		assert.NotEmpty(t, string(body))

		// test rename container
		req, err = http.NewRequest(http.MethodPost, srv.URL+"/api/containers/test-container/rename/new-container?build=true", nil)
		assert.NoError(t, err)
		req.SetBasicAuth("user", "user")

		// send request
		resp, err = http.DefaultTransport.RoundTrip(req)
		assert.NoError(t, err)

		// validate the resp
		body, err = io.ReadAll(resp.Body)
		assert.NoError(t, resp.Body.Close())
		assert.NoError(t, err)
		assert.NotEmpty(t, string(body))

		// test delete container (fail because of no enough permission)
		req, err = http.NewRequest(http.MethodDelete, srv.URL+"/api/containers/new-container", nil)
		assert.NoError(t, err)
		req.SetBasicAuth("user", "user")

		// send request
		resp, err = http.DefaultTransport.RoundTrip(req)
		assert.NoError(t, err)

		// validate the resp
		body, err = io.ReadAll(resp.Body)
		assert.Equal(t, resp.StatusCode, http.StatusUnauthorized)
		assert.NoError(t, err)
		assert.Contains(t, string(body), "Unauthorized")

		// test delete container
		req, err = http.NewRequest(http.MethodDelete, srv.URL+"/api/containers/new-container", nil)
		assert.NoError(t, err)
		req.SetBasicAuth("admin", "admin")

		// send request
		resp, err = http.DefaultTransport.RoundTrip(req)
		assert.NoError(t, err)

		// validate the resp
		body, err = io.ReadAll(resp.Body)
		assert.Equal(t, resp.StatusCode, http.StatusOK)
		assert.NoError(t, err)
		assert.NotEmpty(t, string(body))
	})
}
