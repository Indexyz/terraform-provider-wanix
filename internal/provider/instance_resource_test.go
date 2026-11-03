// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// This server rejects every PATCH and checks full replacement semantics, so a
// regression to server-side apply cannot silently pass the lifecycle test.
func TestInstanceLifecycle(t *testing.T) {
	var mu sync.Mutex
	var object map[string]interface{}
	updates, conflicts := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		notFound := func() {
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"Status","reason":"NotFound","code":404}`))
		}
		switch r.Method {
		case http.MethodPost:
			if object != nil {
				t.Error("unexpected duplicate create")
				w.WriteHeader(409)
				return
			}
			if err := json.NewDecoder(r.Body).Decode(&object); err != nil {
				t.Error(err)
				return
			}
			if object["apiVersion"] != instanceAPIVersion || object["kind"] != instanceKind {
				t.Error("incorrect CRD identity")
			}
			metadata := requireType[map[string]interface{}](t, object["metadata"])
			metadata["resourceVersion"] = "1"
			metadata["finalizers"] = []interface{}{"instance.pop.wanix.net/finalizer"}
			metadata["annotations"] = map[string]interface{}{"operator.example/owned": "keep"}
			object["status"] = map[string]interface{}{"phase": "Ready", "podIPs": []interface{}{"10.0.0.1"}}
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(object)
		case http.MethodGet:
			if object == nil {
				notFound()
				return
			}
			_ = json.NewEncoder(w).Encode(object)
		case http.MethodPut:
			if object == nil {
				notFound()
				return
			}
			if conflicts == 0 {
				conflicts++
				w.WriteHeader(409)
				_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"Status","reason":"Conflict","code":409}`))
				return
			}
			var next map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&next); err != nil {
				t.Error(err)
				return
			}
			metadata := requireType[map[string]interface{}](t, next["metadata"])
			if metadata["resourceVersion"] == nil {
				t.Error("update lost resourceVersion")
			}
			if metadata["finalizers"] == nil {
				t.Error("update lost operator finalizers")
			}
			annotations, _ := metadata["annotations"].(map[string]interface{})
			if annotations["operator.example/owned"] != "keep" {
				t.Error("update lost operator annotation")
			}
			labels, _ := metadata["labels"].(map[string]interface{})
			if _, found := labels["remove"]; found {
				t.Error("removed label still present")
			}
			spec := requireType[map[string]interface{}](t, next["spec"])
			if _, found := spec["images"]; found {
				t.Error("removed images field still present")
			}
			if spec["pause"] != true {
				t.Error("pause was not updated")
			}
			if spec["workloadKind"] != "StatefulSet" {
				t.Error("snake_case field was not converted")
			}
			interfaces := requireType[map[string]interface{}](t, spec["interfaces"])
			iface := requireType[map[string]interface{}](t, interfaces["eth0"])
			if _, found := iface["routes"]; found {
				t.Error("removed nested routes still present")
			}
			metadata["resourceVersion"] = "2"
			object = next
			updates++
			_ = json.NewEncoder(w).Encode(object)
		case http.MethodDelete:
			object = nil
			_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"Status","status":"Success"}`))
		default:
			t.Errorf("unexpected method %s (PATCH must never be used)", r.Method)
			w.WriteHeader(405)
		}
	}))
	defer server.Close()

	config := func(updated bool) string {
		extra := `images = { core = "registry.example/core:v1" }`
		routes := `routes = [{ destination = "10.1.0.0/16", gateway = "192.0.2.1" }]`
		labels := `labels = { keep = "yes", remove = "old" }`
		if updated {
			extra = ""
			routes = ""
			labels = `labels = { keep = "yes" }`
		}
		return fmt.Sprintf(`
provider "wanix" { host = %q }
resource "wanix_instance" "test" {
 name = "test"
 namespace = "default"
 %s
 spec = {
  cpu = "100m"
  pause = %t
  disable_workload = false
  workload_kind = "StatefulSet"
  interfaces = {
   eth0 = { table = 222, address = "192.0.2.10", use_gateway = false, %s }
  }
  %s
 }
}
`, server.URL, labels, updated, routes, extra)
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config(false), Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("wanix_instance.test", "id", "default/test"),
				resource.TestCheckResourceAttr("wanix_instance.test", "pod_ips.0", "10.0.0.1"),
			)},
			{Config: config(true), Check: resource.TestCheckResourceAttr("wanix_instance.test", "spec.pause", "true")},
			{ResourceName: "wanix_instance.test", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"annotations"}},
		},
	})
	mu.Lock()
	defer mu.Unlock()
	if updates != 1 || conflicts != 1 {
		t.Errorf("expected one successful PUT and a retried conflict, got %d updates/%d conflicts", updates, conflicts)
	}
	if object != nil {
		t.Error("resource not deleted")
	}
}
