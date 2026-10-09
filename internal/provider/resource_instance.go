package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hostereo/terraform-provider-hostereo/internal/client"
)

var (
	_ resource.Resource                     = (*instanceResource)(nil)
	_ resource.ResourceWithConfigure        = (*instanceResource)(nil)
	_ resource.ResourceWithImportState      = (*instanceResource)(nil)
	_ resource.ResourceWithConfigValidators = (*instanceResource)(nil)
)

// NewInstanceResource is the hostereo_instance resource.
func NewInstanceResource() resource.Resource { return &instanceResource{} }

type instanceResource struct {
	client *client.Client
}

type instanceModel struct {
	ID                   types.String   `tfsdk:"id"`
	Name                 types.String   `tfsdk:"name"`
	Region               types.String   `tfsdk:"region"`
	PlanID               types.String   `tfsdk:"plan_id"`
	ImageID              types.String   `tfsdk:"image_id"`
	ApplicationVersionID types.String   `tfsdk:"application_version_id"`
	AppFields            types.Map      `tfsdk:"app_fields"`
	Password             types.String   `tfsdk:"password"`
	SSHKeyIDs            types.List     `tfsdk:"ssh_key_ids"`
	Billing              types.String   `tfsdk:"billing"`
	ProjectID            types.String   `tfsdk:"project_id"`
	TagIDs               types.Set      `tfsdk:"tag_ids"`
	PowerState           types.String   `tfsdk:"power_state"`
	DeletionProtection   types.Bool     `tfsdk:"deletion_protection"`
	KeepBackupsOnDestroy types.Bool     `tfsdk:"keep_backups_on_destroy"`
	SnapshotOnDestroy    types.Bool     `tfsdk:"snapshot_on_destroy"`
	IPv4                 types.String   `tfsdk:"ipv4"`
	Status               types.String   `tfsdk:"status"`
	Locked               types.Bool     `tfsdk:"locked"`
	CreatedAt            types.String   `tfsdk:"created_at"`
	RegionID             types.String   `tfsdk:"region_id"`
	PlanName             types.String   `tfsdk:"plan_name"`
	CPU                  types.Int64    `tfsdk:"cpu"`
	MemoryMB             types.Int64    `tfsdk:"memory_mb"`
	DiskGB               types.Int64    `tfsdk:"disk_gb"`
	Timeouts             timeouts.Value `tfsdk:"timeouts"`
}

func (r *instanceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_instance"
}

func (r *instanceResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	createOnly := "Only used when the instance is created; changing it later is ignored (a warning is shown)."
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Hostereo VPS instance.\n\n" +
			"Instances are **deletion-protected by default** (a server-side flag, exposed read-only as `deletion_protection`). " +
			"`terraform destroy` fails until protection is turned off in the Hostereo console (instance Overview → API delete protection). " +
			"Deleting also needs an API key with the `instances:delete` scope.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Instance UUID.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Hostname (letters, digits and hyphens, max 63). Changing it replaces the instance, since the API has no rename.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 63),
				},
			},
			"region": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Region (datacenter) slug or name from `hostereo_regions`, e.g. `tlv-dc-1`. Changing it replaces the instance.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"plan_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Plan UUID from `hostereo_plans`. Changing it resizes the instance in place (the VM is hard-restarted; disks never shrink).",
			},
			"image_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "OS image UUID from `hostereo_images`. Exactly one of `image_id` or `application_version_id`. Changing it replaces the instance.",
				PlanModifiers:       []planmodifier.String{replaceIfKnownString()},
			},
			"application_version_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Application version (`marketplace_version_id`) from `hostereo_applications`. Changing it replaces the instance.",
				PlanModifiers:       []planmodifier.String{replaceIfKnownString()},
			},
			"app_fields": schema.MapAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Install fields for the application (see `fields` in `hostereo_applications`). Changing them replaces the instance.",
				PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplaceIf(func(_ context.Context, req planmodifier.MapRequest, resp *mapplanmodifier.RequiresReplaceIfFuncResponse) {
					resp.RequiresReplace = !req.StateValue.IsNull()
				}, "Replaces the instance when changed after creation.", "Replaces the instance when changed after creation.")},
			},
			"password": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				WriteOnly:           true,
				MarkdownDescription: "Root password (12–50 chars, upper, lower, digit and symbol). Required unless `ssh_key_ids` is set. **Write-only**: sent once at creation and never stored in Terraform state or plan (needs Terraform 1.11+). Changing it later does nothing.",
			},
			"ssh_key_ids": schema.ListAttribute{
				ElementType:         types.Int64Type,
				Optional:            true,
				MarkdownDescription: "Ids of SSH keys saved on your account. Required unless `password` is set. " + createOnly,
			},
			"billing": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("prepaid_monthly"),
				MarkdownDescription: "`prepaid_monthly` (default, paid from balance) or `postpaid`. " + createOnly,
				Validators:          []validator.String{stringvalidator.OneOf("prepaid_monthly", "postpaid")},
			},
			"project_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Project UUID (`hostereo_project`). Updated in place.",
			},
			"tag_ids": schema.SetAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Tag UUIDs (`hostereo_tag`), at most 25. Updated in place.",
			},
			"power_state": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "`running` or `stopped`. When set, Terraform starts or stops the instance to match.",
				Validators:          []validator.String{stringvalidator.OneOf("running", "stopped")},
			},
			"deletion_protection": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Read-only. Server-side protection against deletion, on by default for every instance. It can only be turned off in the Hostereo console (instance Overview → API delete protection); while it is on, `terraform destroy` fails.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"keep_backups_on_destroy": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Keep the instance's backups when it is destroyed (default `true`, like the API). Kept backups remain billed until you delete them in the console.",
			},
			"snapshot_on_destroy": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Take a final snapshot before the instance is destroyed (default `false`).",
			},
			"ipv4": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The instance's public IPv4 as shown in the console (reserved IPs not included). Null until an address is assigned.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"status":     schema.StringAttribute{Computed: true, MarkdownDescription: "Platform status, e.g. `available`, `stopped`, `processing`."},
			"locked":     schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the instance is locked (e.g. unpaid invoice)."},
			"created_at": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"region_id":  schema.StringAttribute{Computed: true, MarkdownDescription: "Region UUID."},
			"plan_name":  schema.StringAttribute{Computed: true},
			"cpu":        schema.Int64Attribute{Computed: true},
			"memory_mb":  schema.Int64Attribute{Computed: true},
			"disk_gb":    schema.Int64Attribute{Computed: true},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true, Update: true, Delete: true,
			}),
		},
	}
}

func (r *instanceResource) ConfigValidators(context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.ExactlyOneOf(path.MatchRoot("image_id"), path.MatchRoot("application_version_id")),
		resourcevalidator.AtLeastOneOf(path.MatchRoot("password"), path.MatchRoot("ssh_key_ids")),
	}
}

func (r *instanceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, true, &resp.Diagnostics)
}

func (r *instanceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ---------------------------------------------------------------- create

func (r *instanceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan instanceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || r.client == nil {
		return
	}
	timeout, d := plan.Timeouts.Create(ctx, 30*time.Minute)
	resp.Diagnostics.Append(d...)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	body := client.CreateInstanceRequest{
		PlanID:               plan.PlanID.ValueString(),
		Region:               plan.Region.ValueString(),
		ImageID:              plan.ImageID.ValueString(),
		MarketplaceVersionID: plan.ApplicationVersionID.ValueString(),
		Name:                 plan.Name.ValueString(),
		Billing:              plan.Billing.ValueString(),
	}
	// password is write-only: it is only present in the configuration.
	var pw types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("password"), &pw)...)
	body.Password = pw.ValueString()
	plan.Password = types.StringNull()
	if !plan.SSHKeyIDs.IsNull() && !plan.SSHKeyIDs.IsUnknown() {
		resp.Diagnostics.Append(plan.SSHKeyIDs.ElementsAs(ctx, &body.SSHKeyIDs, false)...)
	}
	if !plan.AppFields.IsNull() && !plan.AppFields.IsUnknown() {
		resp.Diagnostics.Append(plan.AppFields.ElementsAs(ctx, &body.AppFields, false)...)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	op, err := r.client.CreateInstance(ctx, body, newIdempotencyKey())
	if err != nil {
		apiErr(&resp.Diagnostics, "Creating Hostereo instance failed", err)
		return
	}
	if op.InstanceID == "" {
		resp.Diagnostics.AddError("Creating Hostereo instance failed", "the API accepted the request but returned no instance id")
		return
	}
	// Save the id straight away: if waiting fails, Terraform keeps the
	// instance in state (tainted) instead of losing track of a billed VM.
	plan.ID = types.StringValue(op.InstanceID)
	fillUnknownComputed(&plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)

	if _, err := r.client.WaitOperation(ctx, op); err != nil {
		apiErr(&resp.Diagnostics, "Hostereo instance creation did not complete", err)
		return
	}

	// Post-create settings.
	if pid := plan.ProjectID.ValueString(); pid != "" {
		if err := r.client.SetProject(ctx, op.InstanceID, pid); err != nil {
			apiErr(&resp.Diagnostics, "Assigning the project failed", err)
			return
		}
	}
	if tags := setStrings(ctx, plan.TagIDs, &resp.Diagnostics); len(tags) > 0 {
		if err := r.client.SetTags(ctx, op.InstanceID, tags); err != nil {
			apiErr(&resp.Diagnostics, "Assigning tags failed", err)
			return
		}
	}
	if plan.PowerState.ValueString() == "stopped" {
		if err := r.power(ctx, op.InstanceID, "stopped"); err != nil {
			apiErr(&resp.Diagnostics, "Stopping the new instance failed", err)
			return
		}
	}

	r.refresh(ctx, &plan, &resp.Diagnostics, false)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// ---------------------------------------------------------------- read

func (r *instanceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state instanceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || r.client == nil {
		return
	}
	if gone := r.refresh(ctx, &state, &resp.Diagnostics, true); gone {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// refresh reads the instance, its live power state and organisation into m.
// It returns true when the instance no longer exists.
func (r *instanceResource) refresh(ctx context.Context, m *instanceModel, diags *diagnosticsT, allowGone bool) bool {
	id := m.ID.ValueString()
	inst, err := r.client.GetInstance(ctx, id)
	if err != nil {
		if allowGone && client.IsNotFound(err) {
			return true
		}
		apiErr(diags, "Reading Hostereo instance failed", err)
		return false
	}
	m.Name = types.StringValue(inst.Name)
	m.Status = types.StringValue(inst.Status)
	m.Locked = types.BoolValue(inst.Locked)
	m.CreatedAt = types.StringValue(inst.CreatedAt)
	if inst.Region != nil {
		m.RegionID = types.StringValue(inst.Region.ID)
		// Keep whatever form (slug or name) the user configured, if it matches.
		cur := strings.ToLower(m.Region.ValueString())
		if m.Region.IsNull() || m.Region.IsUnknown() || (cur != strings.ToLower(inst.Region.Slug) && cur != strings.ToLower(inst.Region.Name) && cur != inst.Region.ID) {
			if inst.Region.Slug != "" {
				m.Region = types.StringValue(inst.Region.Slug)
			} else {
				m.Region = types.StringValue(inst.Region.Name)
			}
		}
	} else if m.RegionID.IsUnknown() {
		m.RegionID = types.StringNull()
	}
	if inst.Plan != nil {
		m.PlanID = types.StringValue(inst.Plan.ID)
		m.PlanName = types.StringValue(inst.Plan.Name)
		m.CPU = types.Int64Value(inst.Plan.CPU)
		m.MemoryMB = types.Int64Value(inst.Plan.MemoryMB)
		m.DiskGB = types.Int64Value(inst.Plan.DiskGB)
	}
	if inst.DeletionProtection != nil {
		m.DeletionProtection = types.BoolValue(*inst.DeletionProtection)
	} else if m.DeletionProtection.IsUnknown() {
		m.DeletionProtection = types.BoolNull()
	}
	m.IPv4 = types.StringNull()
	if inst.IPv4 != nil && *inst.IPv4 != "" {
		m.IPv4 = types.StringValue(*inst.IPv4)
	}
	// Destroy-time options are local; after an import they are unset, so use the defaults.
	if m.KeepBackupsOnDestroy.IsNull() || m.KeepBackupsOnDestroy.IsUnknown() {
		m.KeepBackupsOnDestroy = types.BoolValue(true)
	}
	if m.SnapshotOnDestroy.IsNull() || m.SnapshotOnDestroy.IsUnknown() {
		m.SnapshotOnDestroy = types.BoolValue(false)
	}

	if st, err := r.client.GetInstanceStatus(ctx, id); err == nil {
		switch st.Status {
		case "running", "stopped":
			m.PowerState = types.StringValue(st.Status)
		}
	} else if !client.IsNotFound(err) {
		diags.AddWarning("Could not read live power state", err.Error())
	}
	if m.PowerState.IsUnknown() {
		m.PowerState = types.StringNull()
	}

	if org, err := r.client.GetOrganisation(ctx, id); err == nil {
		if org.Project != nil {
			m.ProjectID = types.StringValue(org.Project.ID)
		} else {
			m.ProjectID = types.StringNull()
		}
		if len(org.Tags) == 0 && (m.TagIDs.IsNull() || m.TagIDs.IsUnknown()) {
			m.TagIDs = types.SetNull(types.StringType)
		} else {
			vals := make([]attr.Value, 0, len(org.Tags))
			for _, t := range org.Tags {
				vals = append(vals, types.StringValue(t.ID))
			}
			m.TagIDs = types.SetValueMust(types.StringType, vals)
		}
	} else {
		diags.AddWarning("Could not read project and tags", err.Error())
	}
	fillUnknownComputed(m)
	return false
}

// ---------------------------------------------------------------- update

func (r *instanceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state instanceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || r.client == nil {
		return
	}
	timeout, d := plan.Timeouts.Update(ctx, 30*time.Minute)
	resp.Diagnostics.Append(d...)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	id := state.ID.ValueString()
	plan.ID = state.ID

	for _, f := range []struct {
		name    string
		changed bool
	}{
		{"ssh_key_ids", !state.SSHKeyIDs.IsNull() && !plan.SSHKeyIDs.Equal(state.SSHKeyIDs)},
		{"billing", !state.Billing.IsNull() && !plan.Billing.Equal(state.Billing)},
	} {
		if f.changed {
			resp.Diagnostics.AddAttributeWarning(path.Root(f.name), "Change not applied to the instance",
				fmt.Sprintf("%s is only used when an instance is created. The new value is stored in state but the running instance is not changed.", f.name))
		}
	}

	plan.Password = types.StringNull()
	if !plan.PlanID.Equal(state.PlanID) {
		op, err := r.client.ResizeInstance(ctx, id, plan.PlanID.ValueString())
		if err == nil {
			_, err = r.client.WaitOperation(ctx, op)
		}
		if err != nil {
			apiErr(&resp.Diagnostics, "Resizing the instance failed", err)
			return
		}
	}
	if !plan.ProjectID.Equal(state.ProjectID) {
		if err := r.client.SetProject(ctx, id, plan.ProjectID.ValueString()); err != nil {
			apiErr(&resp.Diagnostics, "Changing the project failed", err)
			return
		}
	}
	if !plan.TagIDs.Equal(state.TagIDs) {
		if err := r.client.SetTags(ctx, id, setStrings(ctx, plan.TagIDs, &resp.Diagnostics)); err != nil {
			apiErr(&resp.Diagnostics, "Changing tags failed", err)
			return
		}
	}
	if want := plan.PowerState.ValueString(); want != "" && !plan.PowerState.IsUnknown() && want != state.PowerState.ValueString() {
		if err := r.power(ctx, id, want); err != nil {
			apiErr(&resp.Diagnostics, "Changing the power state failed", err)
			return
		}
	}

	r.refresh(ctx, &plan, &resp.Diagnostics, false)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// power starts or stops the instance and waits for the operation.
func (r *instanceResource) power(ctx context.Context, id, want string) error {
	action := "start"
	if want == "stopped" {
		action = "stop"
	}
	op, err := r.client.InstanceAction(ctx, id, action)
	if err != nil {
		if client.HasCode(err, "invalid_state") {
			return nil // already in the wanted state
		}
		return err
	}
	_, err = r.client.WaitOperation(ctx, op)
	return err
}

// ---------------------------------------------------------------- delete

func (r *instanceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state instanceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || r.client == nil {
		return
	}
	timeout, d := state.Timeouts.Delete(ctx, 20*time.Minute)
	resp.Diagnostics.Append(d...)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	id := state.ID.ValueString()

	// The confirmation must be the instance's current name.
	name := state.Name.ValueString()
	if inst, err := r.client.GetInstance(ctx, id); err == nil {
		name = inst.Name
	} else if client.IsNotFound(err) {
		return // already gone
	}

	keepBackups := state.KeepBackupsOnDestroy.IsNull() || state.KeepBackupsOnDestroy.IsUnknown() || state.KeepBackupsOnDestroy.ValueBool()
	op, err := r.client.DeleteInstance(ctx, id, name, keepBackups, state.SnapshotOnDestroy.ValueBool())
	if err != nil {
		switch {
		case client.IsNotFound(err):
			return
		case client.HasCode(err, "deletion_protected"):
			resp.Diagnostics.AddError("Instance is deletion-protected",
				fmt.Sprintf("Deletion protection is enabled for %s. Turn it off in the Hostereo console (instance Overview → API delete protection), then run destroy again.", name))
			return
		case client.HasCode(err, "insufficient_scope"):
			resp.Diagnostics.AddError("API key cannot delete instances",
				"Deleting needs an API key with the instances:delete scope. "+err.Error())
			return
		}
		apiErr(&resp.Diagnostics, "Deleting Hostereo instance failed", err)
		return
	}
	if op != nil {
		if _, err := r.client.WaitOperation(ctx, op); err != nil {
			apiErr(&resp.Diagnostics, "Hostereo instance deletion did not complete", err)
			return
		}
	}
	// Wait until the API no longer returns the instance.
	for {
		_, err := r.client.GetInstance(ctx, id)
		if client.IsNotFound(err) {
			return
		}
		var ae *client.APIError
		if err != nil && !(errors.As(err, &ae) && ae.StatusCode >= http.StatusInternalServerError) {
			apiErr(&resp.Diagnostics, "Checking instance deletion failed", err)
			return
		}
		if err := r.client.Sleep(ctx, r.client.PollInterval); err != nil {
			resp.Diagnostics.AddError("Timed out waiting for the instance to be deleted", err.Error())
			return
		}
	}
}

// ---------------------------------------------------------------- helpers

// replaceIfKnownString replaces the instance when a create-time value changes,
// but not when state has no value yet (e.g. right after terraform import, since
// the API does not return the image or application an instance was built from).
func replaceIfKnownString() planmodifier.String {
	return stringplanmodifier.RequiresReplaceIf(func(_ context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
		resp.RequiresReplace = !req.StateValue.IsNull()
	}, "Replaces the instance when changed after creation.", "Replaces the instance when changed after creation.")
}

func setStrings(ctx context.Context, s types.Set, diags *diagnosticsT) []string {
	if s.IsNull() || s.IsUnknown() {
		return nil
	}
	var out []string
	diags.Append(s.ElementsAs(ctx, &out, false)...)
	sort.Strings(out)
	return out
}

// fillUnknownComputed replaces unknown computed values with null so state is
// always known.
func fillUnknownComputed(m *instanceModel) {
	for _, s := range []*types.String{&m.Status, &m.CreatedAt, &m.RegionID, &m.PlanName, &m.PowerState} {
		if s.IsUnknown() {
			*s = types.StringNull()
		}
	}
	for _, i := range []*types.Int64{&m.CPU, &m.MemoryMB, &m.DiskGB} {
		if i.IsUnknown() {
			*i = types.Int64Null()
		}
	}
	if m.Locked.IsUnknown() {
		m.Locked = types.BoolNull()
	}
	if m.DeletionProtection.IsUnknown() {
		m.DeletionProtection = types.BoolNull()
	}
	if m.IPv4.IsUnknown() {
		m.IPv4 = types.StringNull()
	}
}
