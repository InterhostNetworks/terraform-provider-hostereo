package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hostereo/terraform-provider-hostereo/internal/client"
)

const forgetDetail = "The Hostereo API has no delete for %s, so Terraform only removed it from state. It still exists in your account; delete it in the Hostereo console if you no longer need it."

// ============================================================== project

var (
	_ resource.ResourceWithConfigure   = (*projectResource)(nil)
	_ resource.ResourceWithImportState = (*projectResource)(nil)
)

func NewProjectResource() resource.Resource { return &projectResource{} }

type projectResource struct{ client *client.Client }

type projectModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	Description    types.String `tfsdk:"description"`
	InstancesCount types.Int64  `tfsdk:"instances_count"`
}

func (r *projectResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (r *projectResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A project groups instances in the Hostereo console. The API cannot delete projects: `terraform destroy` only removes it from state (with a warning).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Project name (max 64; letters, digits, spaces, underscores, dots, hyphens). Must be unique in your account.",
				Validators:          []validator.String{stringvalidator.LengthBetween(1, 64)},
			},
			"description": schema.StringAttribute{
				Optional:   true,
				Validators: []validator.String{stringvalidator.LengthAtMost(255)},
			},
			"instances_count": schema.Int64Attribute{Computed: true},
		},
	}
}

func (r *projectResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, true, &resp.Diagnostics)
}

func (r *projectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (m *projectModel) fill(p *client.Project) {
	m.ID = types.StringValue(p.ID)
	m.Name = types.StringValue(p.Name)
	if p.Description != nil {
		m.Description = types.StringValue(*p.Description)
	} else {
		m.Description = types.StringNull()
	}
	m.InstancesCount = types.Int64Value(p.InstancesCount)
}

func (m *projectModel) desc() *string {
	if m.Description.IsNull() || m.Description.IsUnknown() {
		return nil
	}
	v := m.Description.ValueString()
	return &v
}

func (r *projectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m projectModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() || r.client == nil {
		return
	}
	p, err := r.client.CreateProject(ctx, m.Name.ValueString(), m.desc())
	if err != nil {
		apiErr(&resp.Diagnostics, "Creating Hostereo project failed", err)
		return
	}
	m.fill(p)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *projectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m projectModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() || r.client == nil {
		return
	}
	p, err := r.client.GetProject(ctx, m.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiErr(&resp.Diagnostics, "Reading Hostereo project failed", err)
		return
	}
	m.fill(p)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *projectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m, state projectModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || r.client == nil {
		return
	}
	p, err := r.client.UpdateProject(ctx, state.ID.ValueString(), m.Name.ValueString(), m.desc())
	if err != nil {
		apiErr(&resp.Diagnostics, "Updating Hostereo project failed", err)
		return
	}
	m.fill(p)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *projectResource) Delete(_ context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.Diagnostics.AddWarning("Project not deleted in Hostereo", sprintf(forgetDetail, "projects"))
}

// ============================================================== tag

var (
	_ resource.ResourceWithConfigure   = (*tagResource)(nil)
	_ resource.ResourceWithImportState = (*tagResource)(nil)
)

func NewTagResource() resource.Resource { return &tagResource{} }

type tagResource struct{ client *client.Client }

type tagModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	InstancesCount types.Int64  `tfsdk:"instances_count"`
}

func (r *tagResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tag"
}

func (r *tagResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A tag for instances. The API cannot delete tags: `terraform destroy` only removes it from state (with a warning).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"name": schema.StringAttribute{
				Required:   true,
				Validators: []validator.String{stringvalidator.LengthBetween(1, 32)},
			},
			"instances_count": schema.Int64Attribute{Computed: true},
		},
	}
}

func (r *tagResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, true, &resp.Diagnostics)
}

func (r *tagResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (m *tagModel) fill(t *client.Tag) {
	m.ID = types.StringValue(t.ID)
	m.Name = types.StringValue(t.Name)
	m.InstancesCount = types.Int64Value(t.InstancesCount)
}

func (r *tagResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m tagModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() || r.client == nil {
		return
	}
	t, err := r.client.CreateTag(ctx, m.Name.ValueString())
	if err != nil {
		apiErr(&resp.Diagnostics, "Creating Hostereo tag failed", err)
		return
	}
	m.fill(t)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *tagResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m tagModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() || r.client == nil {
		return
	}
	t, err := r.client.GetTag(ctx, m.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiErr(&resp.Diagnostics, "Reading Hostereo tag failed", err)
		return
	}
	m.fill(t)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *tagResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m, state tagModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || r.client == nil {
		return
	}
	t, err := r.client.UpdateTag(ctx, state.ID.ValueString(), m.Name.ValueString())
	if err != nil {
		apiErr(&resp.Diagnostics, "Updating Hostereo tag failed", err)
		return
	}
	m.fill(t)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *tagResource) Delete(_ context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.Diagnostics.AddWarning("Tag not deleted in Hostereo", sprintf(forgetDetail, "tags"))
}
