package api

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/warewulf/warewulf/internal/pkg/testenv"
)

func TestOverlayAPI(t *testing.T) {
	env := testenv.New(t)
	defer env.RemoveAll(t)

	t.Run("test all overlay related apis", func(t *testing.T) {
		// prepareration
		srv := httptest.NewServer(Handler())
		defer srv.Close()
		env.WriteFile(t, path.Join(testenv.WWOverlaydir, "/test/file"), `test`)
		env.WriteFile(t, "etc/warewulf/nodes.conf", `nodeprofiles:
  default: {}
nodes:
  n01:
    profiles:
    - default`)
		env.WriteFile(t, "uploadfile", "uploaded")

		// test get all overlays
		req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/overlays", nil)
		assert.NoError(t, err)

		// send request
		resp, err := http.DefaultTransport.RoundTrip(req)
		assert.NoError(t, err)

		// validate the resp
		body, err := io.ReadAll(resp.Body)
		assert.NoError(t, resp.Body.Close())
		assert.NoError(t, err)
		assert.Contains(t, string(body), "\"test\":{\"files\":[\"/file\"]}}")

		// test get single overlay
		req, err = http.NewRequest(http.MethodGet, srv.URL+"/api/overlays/test", nil)
		assert.NoError(t, err)

		// send request
		resp, err = http.DefaultTransport.RoundTrip(req)
		assert.NoError(t, err)

		// validate the resp
		body, err = io.ReadAll(resp.Body)
		assert.NoError(t, resp.Body.Close())
		assert.NoError(t, err)
		assert.Contains(t, string(body), "{\"files\":[\"/file\"]}")

		// test get single file
		req, err = http.NewRequest(http.MethodGet, srv.URL+"/api/overlays/test/files/file", nil)
		assert.NoError(t, err)

		// send request
		resp, err = http.DefaultTransport.RoundTrip(req)
		assert.NoError(t, err)

		// validate the resp
		body, err = io.ReadAll(resp.Body)
		assert.NoError(t, resp.Body.Close())
		assert.NoError(t, err)
		assert.Contains(t, string(body), "{\"overlay\":\"test\",\"path\":\"file\",\"contents\":\"test\"")

		// test create an overlay
		req, err = http.NewRequest(http.MethodPost, srv.URL+"/api/overlays/test-new", nil)
		assert.NoError(t, err)

		// send request
		resp, err = http.DefaultTransport.RoundTrip(req)
		assert.NoError(t, err)

		// validate the resp
		body, err = io.ReadAll(resp.Body)
		assert.NoError(t, resp.Body.Close())
		assert.NoError(t, err)
		assert.Contains(t, string(body), "{\"files\":null}")

		// test build an overlay
		buildOverlay := `{
		  "nodes": [
		    "n01"
		  ],
		  "overlayNames": [
		    "test"
		  ]
		}`
		req, err = http.NewRequest(http.MethodPost, srv.URL+"/api/overlays/build", bytes.NewReader([]byte(buildOverlay)))
		assert.NoError(t, err)

		// send request
		resp, err = http.DefaultTransport.RoundTrip(req)
		assert.NoError(t, err)

		// validate the resp
		body, err = io.ReadAll(resp.Body)
		assert.NoError(t, resp.Body.Close())
		assert.NoError(t, err)
		assert.Contains(t, string(body), "{\"test\":{\"files\":[\"/file\"]}}")

		// test render an overlay file
		req, err = http.NewRequest(http.MethodGet, srv.URL+"/api/overlays/test/render/file?nodeName=n01", nil)
		assert.NoError(t, err)

		// send request
		resp, err = http.DefaultTransport.RoundTrip(req)
		assert.NoError(t, err)

		// validate the resp
		body, err = io.ReadAll(resp.Body)
		assert.NoError(t, resp.Body.Close())
		assert.NoError(t, err)
		assert.Contains(t, string(body), "\"test\"")

		// test delete an overlay file
		req, err = http.NewRequest(http.MethodDelete, srv.URL+"/api/overlays/test/file", nil)
		assert.NoError(t, err)

		// send request
		resp, err = http.DefaultTransport.RoundTrip(req)
		assert.NoError(t, err)

		// validate the resp
		body, err = io.ReadAll(resp.Body)
		assert.NoError(t, resp.Body.Close())
		assert.NoError(t, err)
		assert.Contains(t, string(body), "{\"files\":[\"/file\"]}")

		// test import a file
		reqBody := &bytes.Buffer{}
		writer := multipart.NewWriter(reqBody)

		// Create a form file and add it to the multipart writer
		file, err := os.Open(env.GetPath("uploadfile"))
		assert.NoError(t, err)
		defer file.Close()

		part, err := writer.CreateFormFile("upload", filepath.Base(file.Name()))
		assert.NoError(t, err)

		_, err = io.Copy(part, file)
		assert.NoError(t, err)

		err = writer.Close()
		assert.NoError(t, err)

		req, err = http.NewRequest(http.MethodPost, srv.URL+"/api/overlays/test/import/upload", reqBody)
		assert.NoError(t, err)
		req.Header.Set("Content-Type", writer.FormDataContentType())

		// send request
		resp, err = http.DefaultTransport.RoundTrip(req)
		assert.NoError(t, err)

		// validate the resp
		body, err = io.ReadAll(resp.Body)
		assert.NoError(t, resp.Body.Close())
		assert.NoError(t, err)
		assert.Contains(t, string(body), "{\"files\":[\"/upload\"]}")
	})
}
