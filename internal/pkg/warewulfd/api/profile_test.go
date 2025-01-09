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

func TestProfileAPI(t *testing.T) {
	env := testenv.New(t)
	defer env.RemoveAll()

	t.Run("test all profiles related apis", func(t *testing.T) {
		// prepareration
		srv := httptest.NewServer(Handler())
		defer srv.Close()

		// test get all profiles
		req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/profiles", nil)
		assert.NoError(t, err)

		// send request
		resp, err := http.DefaultTransport.RoundTrip(req)
		assert.NoError(t, err)

		// validate the resp
		body, err := io.ReadAll(resp.Body)
		assert.NoError(t, resp.Body.Close())
		assert.NoError(t, err)
		assert.Contains(t, string(body), "\"default\":{}")

		// test add a new profile
		testProfile := `{
  "profile":{
    "kernel": {
      "version": "v1.0.0",
      "args": "kernel-args"
    }
  },
  "names": [
    "test"
  ]
}`
		req, err = http.NewRequest(http.MethodPut, srv.URL+"/api/profiles", bytes.NewBuffer([]byte(testProfile)))
		assert.NoError(t, err)

		// send request
		resp, err = http.DefaultTransport.RoundTrip(req)
		assert.NoError(t, err)

		// validate the resp
		body, err = io.ReadAll(resp.Body)
		assert.NoError(t, resp.Body.Close())
		assert.NoError(t, err)
		assert.NotEmpty(t, body)

		// re-read all profiles
		req, err = http.NewRequest(http.MethodGet, srv.URL+"/api/profiles", nil)
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

		// get one specific profile
		req, err = http.NewRequest(http.MethodGet, srv.URL+"/api/profiles/test", nil)
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

		// update the profile
		updateProfile := `{
  "profile":{
    "kernel": {
      "version": "v1.0.1-newversion"
    }
  }
}`
		req, err = http.NewRequest(http.MethodPatch, srv.URL+"/api/profiles/test", bytes.NewBuffer([]byte(updateProfile)))
		assert.NoError(t, err)

		// send request
		resp, err = http.DefaultTransport.RoundTrip(req)
		assert.NoError(t, err)

		// validate the resp
		body, err = io.ReadAll(resp.Body)
		assert.NoError(t, resp.Body.Close())
		assert.NoError(t, err)
		assert.NotEmpty(t, body)

		// get one specific profile
		req, err = http.NewRequest(http.MethodGet, srv.URL+"/api/profiles/test", nil)
		assert.NoError(t, err)

		// send request
		resp, err = http.DefaultTransport.RoundTrip(req)
		assert.NoError(t, err)

		// validate the resp
		body, err = io.ReadAll(resp.Body)
		assert.NoError(t, resp.Body.Close())
		assert.NoError(t, err)
		assert.Contains(t, string(body), "v1.0.1-newversion")

		// test delete profiles
		req, err = http.NewRequest(http.MethodDelete, srv.URL+"/api/profiles/test", nil)
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
