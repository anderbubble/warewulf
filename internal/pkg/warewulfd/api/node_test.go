package api

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/warewulf/warewulf/internal/pkg/testenv"
)

func TestNodeAPI(t *testing.T) {
	env := testenv.New(t)
	defer env.RemoveAll()

	t.Run("test all nodes related apis", func(t *testing.T) {
		// prepareration
		srv := httptest.NewServer(Handler())
		defer srv.Close()

		// test add a new node
		testNode := `{
  "node":{
    "kernel": {
      "version": "v1.0.0",
      "args": "kernel-args"
    }
  }
}`
		req, err := http.NewRequest(http.MethodPut, srv.URL+"/api/nodes/test", bytes.NewBuffer([]byte(testNode)))
		assert.NoError(t, err)

		// send request
		resp, err := http.DefaultTransport.RoundTrip(req)
		assert.NoError(t, err)

		// validate the resp
		body, err := io.ReadAll(resp.Body)
		assert.NoError(t, resp.Body.Close())
		assert.NoError(t, err)
		assert.NotEmpty(t, body)

		// re-read all nodes
		req, err = http.NewRequest(http.MethodGet, srv.URL+"/api/nodes", nil)
		assert.NoError(t, err)

		// send request
		resp, err = http.DefaultTransport.RoundTrip(req)
		assert.NoError(t, err)

		// validate the resp
		body, err = io.ReadAll(resp.Body)
		assert.NoError(t, resp.Body.Close())
		assert.NoError(t, err)
		assert.Contains(t, string(body), "test")
		assert.Contains(t, string(body), "default")

		// get one specific node
		req, err = http.NewRequest(http.MethodGet, srv.URL+"/api/nodes/test", nil)
		assert.NoError(t, err)

		// send request
		resp, err = http.DefaultTransport.RoundTrip(req)
		assert.NoError(t, err)

		// validate the resp
		body, err = io.ReadAll(resp.Body)
		assert.NoError(t, resp.Body.Close())
		assert.NoError(t, err)
		assert.Contains(t, string(body), "kernel-args")
		assert.Contains(t, string(body), "v1.0.0")

		// update the node
		updateNode := `{
  "node":{
    "kernel": {
      "version": "v1.0.1-newversion"
    }
  }
}`
		req, err = http.NewRequest(http.MethodPatch, srv.URL+"/api/nodes/test", bytes.NewBuffer([]byte(updateNode)))
		assert.NoError(t, err)

		// send request
		resp, err = http.DefaultTransport.RoundTrip(req)
		assert.NoError(t, err)

		// validate the resp
		body, err = io.ReadAll(resp.Body)
		assert.NoError(t, resp.Body.Close())
		assert.NoError(t, err)
		assert.NotEmpty(t, body)

		// get one specific node
		req, err = http.NewRequest(http.MethodGet, srv.URL+"/api/nodes/test", nil)
		assert.NoError(t, err)

		// send request
		resp, err = http.DefaultTransport.RoundTrip(req)
		assert.NoError(t, err)

		// validate the resp
		body, err = io.ReadAll(resp.Body)
		assert.NoError(t, resp.Body.Close())
		assert.NoError(t, err)
		assert.Contains(t, string(body), "v1.0.1-newversion")

		// test build all nodes overlays
		req, err = http.NewRequest(http.MethodPost, srv.URL+"/api/nodes/overlays/build", nil)
		assert.NoError(t, err)

		// send request
		resp, err = http.DefaultTransport.RoundTrip(req)
		assert.NoError(t, err)

		// validate the resp
		body, err = io.ReadAll(resp.Body)
		assert.NoError(t, resp.Body.Close())
		assert.NoError(t, err)
		assert.NotEmpty(t, body)

		// test build one node's overlays
		req, err = http.NewRequest(http.MethodPost, srv.URL+"/api/nodes/test/overlays/build", nil)
		assert.NoError(t, err)

		// send request
		resp, err = http.DefaultTransport.RoundTrip(req)
		assert.NoError(t, err)

		// validate the resp
		body, err = io.ReadAll(resp.Body)
		assert.NoError(t, resp.Body.Close())
		assert.NoError(t, err)
		assert.NotEmpty(t, body)

		// test delete nodes
		req, err = http.NewRequest(http.MethodDelete, srv.URL+"/api/nodes/test", nil)
		assert.NoError(t, err)

		// send request
		resp, err = http.DefaultTransport.RoundTrip(req)
		assert.NoError(t, err)

		// validate the resp
		body, err = io.ReadAll(resp.Body)
		assert.NoError(t, resp.Body.Close())
		assert.NoError(t, err)
		assert.NotEmpty(t, body)
	})
}
