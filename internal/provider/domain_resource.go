package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/Cloady/terraform-provider-cloady/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_                 resource.ResourceWithConfigure   = &domainResource{}
	_                 resource.ResourceWithImportState = &domainResource{}
	domainHostPattern                                  = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,}$`)
)

type domainResource struct{ client *client.Client }

type domainResourceModel struct {
	ID          types.String `tfsdk:"id"`
	DomainID    types.String `tfsdk:"domain_id"`
	Workspace   types.String `tfsdk:"workspace"`
	App         types.String `tfsdk:"app"`
	Environment types.String `tfsdk:"environment"`
	Region      types.String `tfsdk:"region"`
	Host        types.String `tfsdk:"host"`
	IsPrimary   types.Bool   `tfsdk:"is_primary"`
	RedirectTo  types.String `tfsdk:"redirect_to"`
	Port        types.Int64  `tfsdk:"port"`
}

type domainRow struct {
	ID         string  `json:"id"`
	Host       string  `json:"host"`
	IsPrimary  bool    `json:"isPrimary"`
	RedirectTo *string `json:"redirectTo"`
	Port       *int64  `json:"port"`
}

func NewDomainResource() resource.Resource { return &domainResource{} }

func (r *domainResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_domain"
}

func (r *domainResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	hostValidators := []validator.String{stringvalidator.RegexMatches(domainHostPattern, "must be a lowercase hostname without a scheme, path, trailing dot, or surrounding whitespace")}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a custom domain for an app instance. Configure its DNS separately. Domain routing is reconciled automatically by Cloady.",
		Attributes: map[string]schema.Attribute{
			"id":        schema.StringAttribute{Computed: true, MarkdownDescription: "Import identity: workspace/app/environment/region/domain_id.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"domain_id": schema.StringAttribute{Computed: true, MarkdownDescription: "Domain UUID returned by the API.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"workspace": childScopeAttribute("Workspace slug."),
			"app":       childScopeAttribute("App slug."),
			"environment": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("production"),
				MarkdownDescription: "App environment: production, preview, or development. Defaults to production.",
				Validators:          []validator.String{stringvalidator.OneOf("production", "preview", "development")},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"region": childScopeAttribute("Region ID of the app instance."),
			"host": schema.StringAttribute{
				Required: true, MarkdownDescription: "Lowercase hostname, such as api.example.com. Changes require replacement.",
				Validators: hostValidators, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"is_primary": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Whether this is the primary domain. Defaults to false. Changes require replacement.",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"redirect_to": schema.StringAttribute{Optional: true, MarkdownDescription: "Optional lowercase target hostname. Omit to remove an existing redirect.", Validators: hostValidators},
			"port": schema.Int64Attribute{
				Optional: true, MarkdownDescription: "Optional application port (1–65535). Omit to use Cloady's default routing. Changes require replacement.",
				Validators: []validator.Int64{int64validator.Between(1, 65535)}, PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
		},
	}
}

func (r *domainResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("Expected *client.Client, got %T.", req.ProviderData))
		return
	}
	r.client = c
}

func (r *domainResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data domainResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := map[string]any{"host": data.Host.ValueString(), "isPrimary": data.IsPrimary.ValueBool(), "redirectTo": data.RedirectTo.ValueStringPointer(), "port": data.Port.ValueInt64Pointer()}
	var result struct {
		Domain domainRow `json:"domain"`
	}
	if err := r.client.Do(ctx, http.MethodPost, data.apiPath("/domains"), body, &result); err != nil {
		resp.Diagnostics.AddError("Unable to create domain", err.Error())
		return
	}
	if result.Domain.ID == "" {
		resp.Diagnostics.AddError("Invalid API response", "The create response did not contain a domain ID.")
		return
	}
	data.setRow(result.Domain)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *domainResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data domainResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var result struct {
		Domains []domainRow `json:"domains"`
	}
	if err := r.client.Do(ctx, http.MethodGet, data.apiPath("/domains"), nil, &result); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read domain", err.Error())
		return
	}
	for _, row := range result.Domains {
		if row.ID == data.DomainID.ValueString() {
			data.setRow(row)
			resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *domainResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data domainResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Explicit null clears the redirect; omitting the key leaves it unchanged.
	body := map[string]any{"redirectTo": data.RedirectTo.ValueStringPointer()}
	var result struct {
		Domain domainRow `json:"domain"`
	}
	if err := r.client.Do(ctx, http.MethodPatch, data.apiPath("/domains/"+url.PathEscape(data.DomainID.ValueString())), body, &result); err != nil {
		resp.Diagnostics.AddError("Unable to update domain", err.Error())
		return
	}
	data.setRow(result.Domain)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *domainResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data domainResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Do(ctx, http.MethodDelete, data.apiPath("/domains/"+url.PathEscape(data.DomainID.ValueString())), nil, nil); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete domain", err.Error())
	}
}

func (r *domainResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	childImportState(ctx, req, resp, "domain_id")
}

func (m domainResourceModel) apiPath(suffix string) string {
	return client.AppPath(m.Workspace.ValueString(), m.App.ValueString(), m.Environment.ValueString(), m.Region.ValueString(), suffix)
}

func (m *domainResourceModel) setRow(row domainRow) {
	m.DomainID = types.StringValue(row.ID)
	m.ID = types.StringValue(strings.Join([]string{m.Workspace.ValueString(), m.App.ValueString(), m.Environment.ValueString(), m.Region.ValueString(), row.ID}, "/"))
	m.Host = types.StringValue(row.Host)
	m.IsPrimary = types.BoolValue(row.IsPrimary)
	m.RedirectTo = types.StringPointerValue(row.RedirectTo)
	m.Port = types.Int64PointerValue(row.Port)
}
