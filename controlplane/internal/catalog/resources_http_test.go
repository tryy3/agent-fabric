package catalog_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/db/dbtest"
)

func TestResourcesHTTP(t *testing.T) {
	store := catalog.Open(dbtest.Open(t))
	store.IdentityPrefix = "dev-"
	srv := httptest.NewServer(catalog.Handler(store))
	defer srv.Close()

	createBody := `{
		"name":"Work",
		"kind":"container",
		"spec":{
			"image":"alpine:3.20",
			"containerName":"work",
			"volumes":[{
				"id":"vol_0123456789abcdef",
				"enabled":true,
				"name":"disk",
				"target":"/workspace",
				"whitelisted":true,
				"read":true,
				"write":true,
				"exec":true
			}]
		}
	}`
	resp, err := http.Post(srv.URL+"/v1/resources", "application/json", strings.NewReader(createBody))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create status %d body %s", resp.StatusCode, body)
	}
	var created catalog.Resource
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if created.Kind != catalog.KindContainer {
		t.Fatalf("kind %q", created.Kind)
	}

	listResp, err := http.Get(srv.URL + "/v1/resources")
	if err != nil {
		t.Fatal(err)
	}
	var list []catalog.Resource
	if err := json.NewDecoder(listResp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list status %d", listResp.StatusCode)
	}
	found := false
	for _, r := range list {
		if r.ID == created.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("list %+v missing %s", list, created.ID)
	}

	patchReq, _ := http.NewRequest(http.MethodPatch, srv.URL+"/v1/resources/"+created.ID, strings.NewReader(`{"spec":{"image":"alpine:3.21"}}`))
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp, err := http.DefaultClient.Do(patchReq)
	if err != nil {
		t.Fatal(err)
	}
	if patchResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(patchResp.Body)
		patchResp.Body.Close()
		t.Fatalf("patch status %d body %s", patchResp.StatusCode, body)
	}
	var patched catalog.Resource
	if err := json.NewDecoder(patchResp.Body).Decode(&patched); err != nil {
		t.Fatal(err)
	}
	patchResp.Body.Close()
	var stored struct {
		Image         string `json:"image"`
		ContainerName string `json:"containerName"`
	}
	if err := json.Unmarshal(patched.Spec, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Image != "alpine:3.21" {
		t.Fatalf("image %q", stored.Image)
	}
	if stored.ContainerName != "dev-work" {
		t.Fatalf("containerName %q", stored.ContainerName)
	}

	missing, err := http.Get(srv.URL + "/v1/resources/res_missing")
	if err != nil {
		t.Fatal(err)
	}
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("missing get status %d", missing.StatusCode)
	}
	var errPayload struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(missing.Body).Decode(&errPayload); err != nil {
		t.Fatal(err)
	}
	missing.Body.Close()
	wantErr := `resource "res_missing" not found`
	if errPayload.Error != wantErr {
		t.Fatalf("error %q want %q", errPayload.Error, wantErr)
	}

	delReq, _ := http.NewRequest(http.MethodDelete, srv.URL+"/v1/resources/"+created.ID, nil)
	delResp, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatal(err)
	}
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status %d", delResp.StatusCode)
	}
	delResp.Body.Close()

	gone, err := http.Get(srv.URL + "/v1/resources/" + created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gone.StatusCode != http.StatusNotFound {
		t.Fatalf("get after delete status %d", gone.StatusCode)
	}
	gone.Body.Close()
}
