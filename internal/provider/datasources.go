package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hostereo/terraform-provider-hostereo/internal/client"
)

func sprintf(f string, a ...any) string { return fmt.Sprintf(f, a...) }

// base gives every data source Configure and the client.
type base struct {
	client     *client.Client
	requireKey bool
}

func (b *base) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	b.client = clientFrom(req.ProviderData, b.requireKey, &resp.Diagnostics)
}

// ============================================================== regions

func NewRegionsDataSource() datasource.DataSource { return &regionsDS{} }

type regionsDS struct{ base }

type regionModel struct {
	ID     types.String `tfsdk:"id"`
	Name   types.String `tfsdk:"name"`
	Slug   types.String `tfsdk:"slug"`
	Active types.Bool   `tfsdk:"active"`
}

func (d *regionsDS) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_regions"
}

func (d *regionsDS) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Active regions (datacenters). Public; no API key needed.",
		Attributes: map[string]schema.Attribute{
			"regions": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"id":     schema.StringAttribute{Computed: true},
					"name":   schema.StringAttribute{Computed: true},
					"slug":   schema.StringAttribute{Computed: true, MarkdownDescription: "Use this as `region` on `hostereo_instance`."},
					"active": schema.BoolAttribute{Computed: true},
				}},
			},
		},
	}
}

func (d *regionsDS) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		return
	}
	regions, err := d.client.ListRegions(ctx)
	if err != nil {
		apiErr(&resp.Diagnostics, "Listing regions failed", err)
		return
	}
	out := make([]regionModel, 0, len(regions))
	for _, r := range regions {
		out = append(out, regionModel{types.StringValue(r.ID), types.StringValue(r.Name), types.StringValue(r.Slug), types.BoolValue(r.Active)})
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("regions"), out)...)
}

// ============================================================== plans

func NewPlansDataSource() datasource.DataSource { return &plansDS{} }

type plansDS struct{ base }

type planModel struct {
	ID              types.String  `tfsdk:"id"`
	Name            types.String  `tfsdk:"name"`
	Type            types.String  `tfsdk:"type"`
	CPU             types.Int64   `tfsdk:"cpu"`
	MemoryMB        types.Int64   `tfsdk:"memory_mb"`
	DiskGB          types.Int64   `tfsdk:"disk_gb"`
	PostpaidMonthly types.Float64 `tfsdk:"postpaid_monthly"`
	PrepaidMonthly  types.Float64 `tfsdk:"prepaid_monthly"`
}

func (d *plansDS) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_plans"
}

func (d *plansDS) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Active plans with prices (USD). Public; no API key needed.",
		Attributes: map[string]schema.Attribute{
			"plans": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"id":               schema.StringAttribute{Computed: true},
					"name":             schema.StringAttribute{Computed: true},
					"type":             schema.StringAttribute{Computed: true},
					"cpu":              schema.Int64Attribute{Computed: true},
					"memory_mb":        schema.Int64Attribute{Computed: true},
					"disk_gb":          schema.Int64Attribute{Computed: true},
					"postpaid_monthly": schema.Float64Attribute{Computed: true},
					"prepaid_monthly":  schema.Float64Attribute{Computed: true},
				}},
			},
		},
	}
}

func (d *plansDS) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		return
	}
	plans, err := d.client.ListPlans(ctx)
	if err != nil {
		apiErr(&resp.Diagnostics, "Listing plans failed", err)
		return
	}
	out := make([]planModel, 0, len(plans))
	for _, p := range plans {
		out = append(out, planModel{
			types.StringValue(p.ID), types.StringValue(p.Name), types.StringValue(p.Type),
			types.Int64Value(p.CPU), types.Int64Value(p.MemoryMB), types.Int64Value(p.DiskGB),
			types.Float64Value(p.Pricing.PostpaidMonthly), types.Float64Value(p.Pricing.PrepaidMonthly),
		})
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("plans"), out)...)
}

// ============================================================== images

func NewImagesDataSource() datasource.DataSource { return &imagesDS{} }

type imagesDS struct{ base }

type imageModel struct {
	ID           types.String `tfsdk:"id"`
	OS           types.String `tfsdk:"os"`
	Version      types.String `tfsdk:"version"`
	Distribution types.String `tfsdk:"distribution"`
	RegionID     types.String `tfsdk:"region_id"`
}

func (d *imagesDS) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_images"
}

func (d *imagesDS) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Active OS images. Each image belongs to one region. Public; no API key needed.",
		Attributes: map[string]schema.Attribute{
			"region_id": schema.StringAttribute{Optional: true, MarkdownDescription: "Only images of this region (UUID from `hostereo_regions`)."},
			"images": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"id":           schema.StringAttribute{Computed: true},
					"os":           schema.StringAttribute{Computed: true},
					"version":      schema.StringAttribute{Computed: true},
					"distribution": schema.StringAttribute{Computed: true},
					"region_id":    schema.StringAttribute{Computed: true},
				}},
			},
		},
	}
}

func (d *imagesDS) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		return
	}
	var region types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("region_id"), &region)...)
	images, err := d.client.ListImages(ctx)
	if err != nil {
		apiErr(&resp.Diagnostics, "Listing images failed", err)
		return
	}
	out := make([]imageModel, 0, len(images))
	for _, i := range images {
		if !region.IsNull() && region.ValueString() != i.DatacenterID {
			continue
		}
		out = append(out, imageModel{types.StringValue(i.ID), types.StringValue(i.OS), types.StringValue(i.Version), types.StringValue(strings.TrimSpace(i.Distribution)), types.StringValue(i.DatacenterID)})
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("region_id"), region)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("images"), out)...)
}

// ============================================================== applications

func NewApplicationsDataSource() datasource.DataSource { return &appsDS{} }

type appsDS struct{ base }

type appVersionModel struct {
	ID        types.String `tfsdk:"id"`
	Version   types.String `tfsdk:"version"`
	OS        types.String `tfsdk:"os"`
	RegionIDs []string     `tfsdk:"region_ids"`
}

type appFieldModel struct {
	Name        types.String `tfsdk:"name"`
	Type        types.String `tfsdk:"type"`
	Required    types.Bool   `tfsdk:"required"`
	Description types.String `tfsdk:"description"`
}

type appModel struct {
	Name     types.String      `tfsdk:"name"`
	Versions []appVersionModel `tfsdk:"versions"`
	Fields   []appFieldModel   `tfsdk:"fields"`
}

func (d *appsDS) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_applications"
}

func (d *appsDS) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Marketplace applications (WordPress, Supabase, WireGuard, …). Use a version `id` as `application_version_id` and the `fields` as `app_fields`. Public; no API key needed.",
		Attributes: map[string]schema.Attribute{
			"applications": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"name": schema.StringAttribute{Computed: true},
					"versions": schema.ListNestedAttribute{Computed: true, NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
						"id":         schema.StringAttribute{Computed: true, MarkdownDescription: "The `marketplace_version_id`."},
						"version":    schema.StringAttribute{Computed: true},
						"os":         schema.StringAttribute{Computed: true},
						"region_ids": schema.ListAttribute{Computed: true, ElementType: types.StringType},
					}}},
					"fields": schema.ListNestedAttribute{Computed: true, NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
						"name":        schema.StringAttribute{Computed: true},
						"type":        schema.StringAttribute{Computed: true},
						"required":    schema.BoolAttribute{Computed: true},
						"description": schema.StringAttribute{Computed: true},
					}}},
				}},
			},
		},
	}
}

func (d *appsDS) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		return
	}
	apps, err := d.client.ListApplications(ctx)
	if err != nil {
		apiErr(&resp.Diagnostics, "Listing applications failed", err)
		return
	}
	out := make([]appModel, 0, len(apps))
	for _, a := range apps {
		m := appModel{Name: types.StringValue(a.Name), Versions: []appVersionModel{}, Fields: []appFieldModel{}}
		for _, v := range a.Versions {
			ids := []string{}
			for _, dc := range v.Datacenters {
				ids = append(ids, dc.ID)
			}
			m.Versions = append(m.Versions, appVersionModel{types.StringValue(v.MarketplaceVersionID), types.StringValue(v.Version), types.StringValue(v.OS), ids})
		}
		for _, f := range a.Fields {
			m.Fields = append(m.Fields, appFieldModel{types.StringValue(f.Name), types.StringValue(f.Type), types.BoolValue(f.Required), types.StringValue(f.Description)})
		}
		out = append(out, m)
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("applications"), out)...)
}

// ============================================================== instance

func NewInstanceDataSource() datasource.DataSource { return &instanceDS{base: base{requireKey: true}} }

type instanceDS struct{ base }

type instanceDSModel struct {
	ID                 types.String `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	Status             types.String `tfsdk:"status"`
	Region             types.String `tfsdk:"region"`
	RegionID           types.String `tfsdk:"region_id"`
	PlanID             types.String `tfsdk:"plan_id"`
	PlanName           types.String `tfsdk:"plan_name"`
	CPU                types.Int64  `tfsdk:"cpu"`
	MemoryMB           types.Int64  `tfsdk:"memory_mb"`
	DiskGB             types.Int64  `tfsdk:"disk_gb"`
	Locked             types.Bool   `tfsdk:"locked"`
	DeletionProtection types.Bool   `tfsdk:"deletion_protection"`
	CreatedAt          types.String `tfsdk:"created_at"`
}

func (d *instanceDS) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_instance"
}

func (d *instanceDS) ConfigValidators(context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name"))}
}

func (d *instanceDS) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up one of your instances by `id` or `name`.",
		Attributes: map[string]schema.Attribute{
			"id":                  schema.StringAttribute{Optional: true, Computed: true},
			"name":                schema.StringAttribute{Optional: true, Computed: true},
			"status":              schema.StringAttribute{Computed: true},
			"region":              schema.StringAttribute{Computed: true, MarkdownDescription: "Region slug."},
			"region_id":           schema.StringAttribute{Computed: true},
			"plan_id":             schema.StringAttribute{Computed: true},
			"plan_name":           schema.StringAttribute{Computed: true},
			"cpu":                 schema.Int64Attribute{Computed: true},
			"memory_mb":           schema.Int64Attribute{Computed: true},
			"disk_gb":             schema.Int64Attribute{Computed: true},
			"locked":              schema.BoolAttribute{Computed: true},
			"deletion_protection": schema.BoolAttribute{Computed: true},
			"created_at":          schema.StringAttribute{Computed: true},
		},
	}
}

func (d *instanceDS) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		return
	}
	var m instanceDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var inst *client.Instance
	if !m.ID.IsNull() {
		i, err := d.client.GetInstance(ctx, m.ID.ValueString())
		if err != nil {
			apiErr(&resp.Diagnostics, "Reading instance failed", err)
			return
		}
		inst = i
	} else {
		all, err := d.client.ListInstances(ctx)
		if err != nil {
			apiErr(&resp.Diagnostics, "Listing instances failed", err)
			return
		}
		var matches []client.Instance
		for _, i := range all {
			if i.Name == m.Name.ValueString() {
				matches = append(matches, i)
			}
		}
		switch len(matches) {
		case 0:
			resp.Diagnostics.AddError("Instance not found", fmt.Sprintf("no instance named %q in this account", m.Name.ValueString()))
			return
		case 1:
			inst = &matches[0]
		default:
			resp.Diagnostics.AddError("Instance name is ambiguous", fmt.Sprintf("%d instances are named %q; look it up by id instead", len(matches), m.Name.ValueString()))
			return
		}
	}
	m.ID, m.Name, m.Status = types.StringValue(inst.ID), types.StringValue(inst.Name), types.StringValue(inst.Status)
	m.Locked, m.CreatedAt = types.BoolValue(inst.Locked), types.StringValue(inst.CreatedAt)
	m.Region, m.RegionID = types.StringNull(), types.StringNull()
	if inst.Region != nil {
		m.Region, m.RegionID = types.StringValue(inst.Region.Slug), types.StringValue(inst.Region.ID)
	}
	m.PlanID, m.PlanName, m.CPU, m.MemoryMB, m.DiskGB = types.StringNull(), types.StringNull(), types.Int64Null(), types.Int64Null(), types.Int64Null()
	if inst.Plan != nil {
		m.PlanID, m.PlanName = types.StringValue(inst.Plan.ID), types.StringValue(inst.Plan.Name)
		m.CPU, m.MemoryMB, m.DiskGB = types.Int64Value(inst.Plan.CPU), types.Int64Value(inst.Plan.MemoryMB), types.Int64Value(inst.Plan.DiskGB)
	}
	m.DeletionProtection = types.BoolNull()
	if inst.DeletionProtection != nil {
		m.DeletionProtection = types.BoolValue(*inst.DeletionProtection)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
