package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestDomainLifecycleClearsRedirect(t *testing.T) {
	ctx := context.Background()
	target := "www.example.com"
	port := int64(8080)
	row := domainRow{ID: testChildUUID, Host: "api.example.com", RedirectTo: &target, Port: &port}
	r := &domainResource{client: childTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		assertChildScope(t, req)
		switch req.Method {
		case http.MethodPost:
			var body map[string]any
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["host"] != row.Host || body["isPrimary"] != false || body["port"] != float64(8080) || body["redirectTo"] != target {
				t.Errorf("unexpected create body: %#v", body)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"domain": row})
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"domains": []domainRow{row}})
		case http.MethodPatch:
			var body map[string]any
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			value, exists := body["redirectTo"]
			if !exists || value != nil || len(body) != 1 {
				t.Errorf("clearing redirect must send only explicit null: %#v", body)
			}
			row.RedirectTo = nil
			_ = json.NewEncoder(w).Encode(map[string]any{"domain": row})
		case http.MethodDelete:
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Errorf("unexpected request: %s %s", req.Method, req.URL)
			w.WriteHeader(http.StatusBadRequest)
		}
	})}
	model := domainResourceModel{ID: types.StringUnknown(), DomainID: types.StringUnknown(), Workspace: types.StringValue("team"), App: types.StringValue("api"), Environment: types.StringValue("production"), Region: types.StringValue("us-east"), Host: types.StringValue(row.Host), IsPrimary: types.BoolValue(false), RedirectTo: types.StringValue(target), Port: types.Int64Value(port)}
	state := childTestState(t, r, &model)
	created := resource.CreateResponse{State: tfsdk.State{Schema: state.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: state.Schema, Raw: state.Raw}}, &created)
	if created.Diagnostics.HasError() {
		t.Fatal(created.Diagnostics)
	}
	read := resource.ReadResponse{State: created.State}
	r.Read(ctx, resource.ReadRequest{State: created.State}, &read)
	if read.Diagnostics.HasError() {
		t.Fatal(read.Diagnostics)
	}
	if diags := read.State.Get(ctx, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if model.DomainID.ValueString() != testChildUUID {
		t.Fatal("domain ID was not saved")
	}
	model.RedirectTo = types.StringNull()
	planned := childTestState(t, r, &model)
	updated := resource.UpdateResponse{State: read.State}
	r.Update(ctx, resource.UpdateRequest{State: read.State, Plan: tfsdk.Plan{Schema: planned.Schema, Raw: planned.Raw}}, &updated)
	if updated.Diagnostics.HasError() {
		t.Fatal(updated.Diagnostics)
	}
	if diags := updated.State.Get(ctx, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if !model.RedirectTo.IsNull() {
		t.Fatal("redirect remained in state after clearing")
	}
	deleted := resource.DeleteResponse{State: updated.State}
	r.Delete(ctx, resource.DeleteRequest{State: updated.State}, &deleted)
	if deleted.Diagnostics.HasError() {
		t.Fatal(deleted.Diagnostics)
	}
}

func TestDomainImportMissingRemovesState(t *testing.T) {
	ctx := context.Background()
	r := &domainResource{client: childTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		assertChildScope(t, req)
		_ = json.NewEncoder(w).Encode(map[string]any{"domains": []domainRow{}})
	})}
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	// Terraform hands ImportState a null object of the schema type; without a Raw
	// value SetAttribute cannot build one and every write fails.
	imported := resource.ImportStateResponse{State: tfsdk.State{Schema: schemaResp.Schema,
		Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil)}}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "team/api/production/us-east/" + testChildUUID}, &imported)
	if imported.Diagnostics.HasError() {
		t.Fatal(imported.Diagnostics)
	}
	read := resource.ReadResponse{State: imported.State}
	r.Read(ctx, resource.ReadRequest{State: imported.State}, &read)
	if read.Diagnostics.HasError() {
		t.Fatal(read.Diagnostics)
	}
	if !read.State.Raw.IsNull() {
		t.Fatal("missing imported domain must be removed from state")
	}
}

func TestDomainCanonicalHostname(t *testing.T) {
	for _, host := range []string{"api.example.com", "a-b.example.co.uk", "1.example.com"} {
		if !domainHostPattern.MatchString(host) {
			t.Errorf("valid host rejected: %s", host)
		}
	}
	for _, host := range []string{"API.example.com", " api.example.com", "api.example.com.", "https://api.example.com", "example.com/path", "-api.example.com", "*.example.com", "localhost", "127.0.0.1"} {
		if domainHostPattern.MatchString(host) {
			t.Errorf("invalid/noncanonical host accepted: %s", host)
		}
	}
}
