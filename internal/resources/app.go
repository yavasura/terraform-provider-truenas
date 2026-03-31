package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"time"

	customtypes "github.com/deevus/terraform-provider-truenas/internal/types"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"gopkg.in/yaml.v3"
)

var _ resource.Resource = &AppResource{}
var _ resource.ResourceWithConfigure = &AppResource{}
var _ resource.ResourceWithImportState = &AppResource{}
var _ resource.ResourceWithValidateConfig = &AppResource{}

var appNameRegexp = regexp.MustCompile(`^[a-z]([-a-z0-9]*[a-z0-9])?$`)

const (
	defaultAppTrain                 = "stable"
	defaultAppVersion               = "latest"
	defaultAppStateTimeout          = 120 * time.Second
	appWriteOnlyMaskPrivateStateKey = "app_write_only_values_mask"
)

// AppResource defines the resource implementation.
type AppResource struct {
	BaseResource
}

// AppResourceModel describes the resource data model.
type AppResourceModel struct {
	ID                        types.String                           `tfsdk:"id"`
	Name                      types.String                           `tfsdk:"name"`
	CustomApp                 types.Bool                             `tfsdk:"custom_app"`
	CatalogApp                types.String                           `tfsdk:"catalog_app"`
	Train                     types.String                           `tfsdk:"train"`
	Version                   types.String                           `tfsdk:"version"`
	Values                    types.Dynamic                          `tfsdk:"values"`
	WriteOnlyValues           types.Dynamic                          `tfsdk:"write_only_values"`
	CustomComposeConfig       types.Dynamic                          `tfsdk:"custom_compose_config"`
	CustomComposeConfigString customtypes.YAMLStringValue            `tfsdk:"custom_compose_config_string"`
	ComposeConfig             customtypes.YAMLStringValue            `tfsdk:"compose_config"`
	DesiredState              customtypes.CaseInsensitiveStringValue `tfsdk:"desired_state"`
	StateTimeout              types.Int64                            `tfsdk:"state_timeout"`
	State                     types.String                           `tfsdk:"state"`
	UpgradeAvailable          types.Bool                             `tfsdk:"upgrade_available"`
	LatestVersion             types.String                           `tfsdk:"latest_version"`
	LatestAppVersion          types.String                           `tfsdk:"latest_app_version"`
	ImageUpdatesAvailable     types.Bool                             `tfsdk:"image_updates_available"`
	Migrated                  types.Bool                             `tfsdk:"migrated"`
	HumanVersion              types.String                           `tfsdk:"human_version"`
	InstalledVersion          types.String                           `tfsdk:"installed_version"`
	Metadata                  types.Dynamic                          `tfsdk:"metadata"`
	ActiveWorkloads           types.Dynamic                          `tfsdk:"active_workloads"`
	Notes                     types.String                           `tfsdk:"notes"`
	Portals                   types.Dynamic                          `tfsdk:"portals"`
	VersionDetails            types.Dynamic                          `tfsdk:"version_details"`
	Config                    types.Dynamic                          `tfsdk:"config"`
	RestartTriggers           types.Map                              `tfsdk:"restart_triggers"`
}

type appUserStateSnapshot struct {
	DesiredState              customtypes.CaseInsensitiveStringValue
	StateTimeout              types.Int64
	RestartTriggers           types.Map
	CatalogApp                types.String
	Train                     types.String
	Version                   types.String
	Values                    types.Dynamic
	WriteOnlyValues           types.Dynamic
	CustomComposeConfig       types.Dynamic
	CustomComposeConfigString customtypes.YAMLStringValue
	ComposeConfig             customtypes.YAMLStringValue
}

// NewAppResource creates a new AppResource.
func NewAppResource() resource.Resource {
	return &AppResource{}
}

func (r *AppResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_app"
}

func (r *AppResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a TrueNAS application using the TrueNAS app.create/app.update API.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Application identifier (the app name).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Application name. Maps to the API's `app_name` field.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 40),
					stringvalidator.RegexMatches(appNameRegexp, "must match ^[a-z]([-a-z0-9]*[a-z0-9])?$"),
				},
			},
			"custom_app": schema.BoolAttribute{
				Description: "Whether this is a custom Docker Compose application (`true`) or a catalog application (`false`).",
				Required:    true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"catalog_app": schema.StringAttribute{
				Description: "Catalog application name. Required when `custom_app` is false.",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"train": schema.StringAttribute{
				Description: "Catalog train to install from. Defaults to `stable`.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"version": schema.StringAttribute{
				Description: "Desired catalog version selector for create/replace operations. Defaults to `latest`.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"values": schema.DynamicAttribute{
				Description: "Application values object passed to the API and stored in Terraform state.",
				Optional:    true,
			},
			"write_only_values": schema.DynamicAttribute{
				Description: "Application values object passed to the API for create and update operations, but omitted from Terraform state. Intended for secrets.",
				Optional:    true,
				WriteOnly:   true,
			},
			"custom_compose_config": schema.DynamicAttribute{
				Description: "Structured Docker Compose configuration object for custom applications.",
				Optional:    true,
			},
			"custom_compose_config_string": schema.StringAttribute{
				Description: "Docker Compose YAML configuration string for custom applications.",
				Optional:    true,
				CustomType:  customtypes.YAMLStringType{},
			},
			"compose_config": schema.StringAttribute{
				Description:        "Deprecated alias for `custom_compose_config_string`.",
				Optional:           true,
				CustomType:         customtypes.YAMLStringType{},
				DeprecationMessage: "Use custom_compose_config_string instead. This alias will be removed in a future major release.",
			},
			"desired_state": schema.StringAttribute{
				Description: "Desired application state: 'running' or 'stopped' (case-insensitive). Defaults to 'RUNNING'.",
				Optional:    true,
				Computed:    true,
				CustomType:  customtypes.CaseInsensitiveStringType{},
				Default:     customtypes.CaseInsensitiveStringDefault("RUNNING"),
				Validators: []validator.String{
					stringvalidator.Any(
						stringvalidator.OneOfCaseInsensitive("running", "stopped"),
					),
				},
			},
			"state_timeout": schema.Int64Attribute{
				Description: "Timeout in seconds to wait for state transitions. Defaults to 120. Range: 30-600.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(120),
				Validators: []validator.Int64{
					int64validator.Between(30, 600),
				},
			},
			"state": schema.StringAttribute{
				Description: "Application state (RUNNING, STOPPED, etc.).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					computedStatePlanModifier(),
				},
			},
			"upgrade_available": schema.BoolAttribute{
				Description: "Whether an upgrade is available for the application.",
				Computed:    true,
			},
			"latest_version": schema.StringAttribute{
				Description: "Latest available version reported by the API, if any.",
				Computed:    true,
			},
			"latest_app_version": schema.StringAttribute{
				Description: "Latest application version reported by the API, if any.",
				Computed:    true,
			},
			"image_updates_available": schema.BoolAttribute{
				Description: "Whether newer container images are available.",
				Computed:    true,
			},
			"migrated": schema.BoolAttribute{
				Description: "Whether the app was migrated from Kubernetes.",
				Computed:    true,
			},
			"human_version": schema.StringAttribute{
				Description: "Human-readable installed version string.",
				Computed:    true,
			},
			"installed_version": schema.StringAttribute{
				Description: "Installed application version reported by the API.",
				Computed:    true,
			},
			"metadata": schema.DynamicAttribute{
				Description: "Application metadata reported by the API.",
				Computed:    true,
			},
			"active_workloads": schema.DynamicAttribute{
				Description: "Active workload details reported by the API.",
				Computed:    true,
			},
			"notes": schema.StringAttribute{
				Description: "Application notes reported by the API.",
				Computed:    true,
			},
			"portals": schema.DynamicAttribute{
				Description: "Application portals reported by the API.",
				Computed:    true,
			},
			"version_details": schema.DynamicAttribute{
				Description: "Detailed version information reported by the API.",
				Computed:    true,
			},
			"config": schema.DynamicAttribute{
				Description: "Current application config returned by the API.",
				Computed:    true,
			},
			"restart_triggers": schema.MapAttribute{
				Description: "Map of values that, when changed, trigger an app restart. " +
					"Use this to restart the app when dependent resources change, e.g., " +
					"`restart_triggers = { config_checksum = truenas_file.config.checksum }`.",
				Optional:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (r *AppResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data AppResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.CustomApp.IsNull() || data.CustomApp.IsUnknown() {
		return
	}

	hasComposeObject := !data.CustomComposeConfig.IsNull() && !data.CustomComposeConfig.IsUnknown()
	hasComposeString := (!data.CustomComposeConfigString.IsNull() && !data.CustomComposeConfigString.IsUnknown() && data.CustomComposeConfigString.ValueString() != "") ||
		(!data.ComposeConfig.IsNull() && !data.ComposeConfig.IsUnknown() && data.ComposeConfig.ValueString() != "")
	hasUnknownCompose := data.CustomComposeConfig.IsUnknown() ||
		data.CustomComposeConfigString.IsUnknown() ||
		data.ComposeConfig.IsUnknown()

	if hasComposeObject && hasComposeString {
		resp.Diagnostics.AddAttributeError(
			path.Root("custom_compose_config"),
			"Conflicting Compose Configuration",
			"Set only one of custom_compose_config, custom_compose_config_string, or compose_config.",
		)
	}

	if data.CustomApp.ValueBool() {
		if !data.CatalogApp.IsNull() && !data.CatalogApp.IsUnknown() && data.CatalogApp.ValueString() != "" {
			resp.Diagnostics.AddAttributeError(
				path.Root("catalog_app"),
				"Unsupported Catalog Field For Custom App",
				"catalog_app must not be set when custom_app is true.",
			)
		}
		if !hasComposeObject && !hasComposeString && !hasUnknownCompose {
			resp.Diagnostics.AddError(
				"Missing Custom App Configuration",
				"Custom apps require one of custom_compose_config, custom_compose_config_string, or compose_config.",
			)
		}
		return
	}

	if data.CatalogApp.IsNull() || data.CatalogApp.IsUnknown() || data.CatalogApp.ValueString() == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("catalog_app"),
			"Missing Catalog App",
			"catalog_app is required when custom_app is false.",
		)
	}

	if hasComposeObject || hasComposeString {
		resp.Diagnostics.AddError(
			"Unsupported Compose Configuration For Catalog App",
			"Catalog apps do not support custom_compose_config, custom_compose_config_string, or compose_config.",
		)
	}
}

func (r *AppResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data AppResourceModel
	var configData AppResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(req.Config.Get(ctx, &configData)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !configData.WriteOnlyValues.IsNull() && !configData.WriteOnlyValues.IsUnknown() {
		data.WriteOnlyValues = configData.WriteOnlyValues
	}

	resp.Diagnostics.Append(storeAppWriteOnlyMask(ctx, resp.Private, data.WriteOnlyValues)...)
	if resp.Diagnostics.HasError() {
		return
	}

	currentWriteOnlyMask := dynamicValueToAny(ctx, data.WriteOnlyValues)

	appName := data.Name.ValueString()
	plannedValues := data.Values
	applyCreateDefaults(&data)

	app, err := r.createAppEntry(ctx, &data)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create App",
			fmt.Sprintf("Unable to create app %q: %s", appName, err.Error()),
		)
		return
	}

	// Map response to model
	app.Config = redactAppConfig(app.Config, currentWriteOnlyMask)
	r.syncAppStateToModel(&data, app, true)
	data.Values = plannedValues

	// Handle desired_state - if user wants STOPPED but app started as RUNNING
	desiredState := data.DesiredState.ValueString()
	if desiredState == "" {
		desiredState = AppStateRunning
	}
	normalizedDesired := normalizeDesiredState(desiredState)

	if app.State != normalizedDesired {
		timeout := appStateTimeout(data.StateTimeout)

		// For Create, we don't warn about drift - it's expected that we may need to stop
		if normalizedDesired == AppStateRunning {
			err = r.services.App.StartApp(ctx, appName)
		} else {
			err = r.services.App.StopApp(ctx, appName)
		}
		if err != nil {
			action := "start"
			if normalizedDesired != AppStateRunning {
				action = "stop"
			}
			resp.Diagnostics.AddError(
				"Unable to Set App State",
				fmt.Sprintf("Unable to %s app %q: %s", action, appName, err.Error()),
			)
			return
		}

		finalState, err := r.waitForAppStableState(ctx, appName, timeout)
		if err != nil {
			resp.Diagnostics.AddError(
				"Timeout Waiting for App State",
				err.Error(),
			)
			return
		}

		data.State = types.StringValue(finalState)
	}

	// Preserve user's original desired_state value (semantic equality handles case differences)
	// Only set if it was empty (defaulting to RUNNING)
	data.DesiredState = defaultDesiredStateValue(data.DesiredState, AppStateRunning)

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AppResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data AppResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	prior := snapshotAppUserState(data)

	writeOnlyMask, diags := loadAppWriteOnlyMask(ctx, req.Private)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Use the name to query the app
	appName := data.Name.ValueString()

	app, err := r.readAppEntry(ctx, appName)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read App",
			fmt.Sprintf("Unable to read app %q: %s", appName, err.Error()),
		)
		return
	}

	// Check if app was found
	if app == nil {
		// App was deleted outside of Terraform - remove from state
		resp.State.RemoveResource(ctx)
		return
	}

	app.Config = redactAppConfig(app.Config, writeOnlyMask)
	r.syncAppStateToModel(&data, app, false)
	prior.restoreRead(&data, app.State)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AppResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data AppResourceModel
	var configData AppResourceModel
	var stateData AppResourceModel
	var diags diag.Diagnostics

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(req.Config.Get(ctx, &configData)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !configData.WriteOnlyValues.IsNull() && !configData.WriteOnlyValues.IsUnknown() {
		data.WriteOnlyValues = configData.WriteOnlyValues
	}

	resp.Diagnostics.Append(storeAppWriteOnlyMask(ctx, resp.Private, data.WriteOnlyValues)...)
	if resp.Diagnostics.HasError() {
		return
	}

	currentWriteOnlyMask := dynamicValueToAny(ctx, data.WriteOnlyValues)
	if currentWriteOnlyMask == nil {
		currentWriteOnlyMask, diags = loadAppWriteOnlyMask(ctx, req.Private)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// Read current state data to detect compose_config changes
	resp.Diagnostics.Append(req.State.Get(ctx, &stateData)...)
	if resp.Diagnostics.HasError() {
		return
	}

	appName := data.Name.ValueString()
	planned := snapshotAppUserState(data)

	// Handle compose_config changes first (if any)
	writeOnlyValuesConfigured := !data.WriteOnlyValues.IsNull() && !data.WriteOnlyValues.IsUnknown()
	composeConfigChanged := writeOnlyValuesConfigured ||
		!data.Values.Equal(stateData.Values) ||
		!data.CustomComposeConfig.Equal(stateData.CustomComposeConfig) ||
		!data.CustomComposeConfigString.Equal(stateData.CustomComposeConfigString) ||
		!data.ComposeConfig.Equal(stateData.ComposeConfig)
	if composeConfigChanged {
		_, err := r.updateAppEntry(ctx, &data)
		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to Update App",
				fmt.Sprintf("Unable to update app %q: %s", appName, err.Error()),
			)
			return
		}
	}

	// Check if restart_triggers changed - if so, we need to restart the app
	restartTriggersChanged := !data.RestartTriggers.Equal(stateData.RestartTriggers)
	needsRestart := restartTriggersChanged && !data.RestartTriggers.IsNull() && !stateData.RestartTriggers.IsNull()

	// Query the app to get current state
	currentState, err := r.queryAppState(ctx, appName)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Query App State",
			fmt.Sprintf("Unable to query app %q state: %s", appName, err.Error()),
		)
		return
	}

	// Get timeout from plan
	timeout := appStateTimeout(data.StateTimeout)

	// Wait for transitional states to complete before reconciling
	if !isStableState(currentState) {
		stableState, err := r.waitForAppStableState(ctx, appName, timeout)
		if err != nil {
			resp.Diagnostics.AddError(
				"Timeout Waiting for App State",
				err.Error(),
			)
			return
		}
		currentState = stableState
	}

	// Handle restart_triggers: if triggers changed and app is running, restart it
	if needsRestart && currentState == AppStateRunning {
		// Restart by stopping then starting the app
		err := r.services.App.StopApp(ctx, appName)
		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to Stop App for Restart",
				fmt.Sprintf("Unable to stop app %q for restart triggered by restart_triggers change: %s", appName, err.Error()),
			)
			return
		}

		err = r.services.App.StartApp(ctx, appName)
		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to Start App for Restart",
				fmt.Sprintf("Unable to start app %q for restart triggered by restart_triggers change: %s", appName, err.Error()),
			)
			return
		}

		stableState, err := r.waitForAppStableState(ctx, appName, timeout)
		if err != nil {
			resp.Diagnostics.AddError(
				"Timeout Waiting for App State After Restart",
				err.Error(),
			)
			return
		}
		currentState = stableState
	}

	// Reconcile desired_state - this adds drift warnings if state was externally changed
	desiredState := data.DesiredState.ValueString()
	if desiredState == "" {
		desiredState = AppStateRunning
	}
	normalizedDesired := normalizeDesiredState(desiredState)

	if currentState != normalizedDesired {
		err := r.reconcileDesiredState(ctx, appName, currentState, normalizedDesired, timeout, resp)
		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to Reconcile App State",
				err.Error(),
			)
			return
		}
		// Query final state after reconciliation
		currentState, err = r.queryAppState(ctx, appName)
		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to Query App State After Reconciliation",
				fmt.Sprintf("Unable to query app %q state: %s", appName, err.Error()),
			)
			return
		}
	}

	// Map final state to model
	app, err := r.readAppEntry(ctx, appName)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read App After Update",
			fmt.Sprintf("Unable to read app %q after update: %s", appName, err.Error()),
		)
		return
	}
	if app == nil {
		data.ID = types.StringValue(appName)
		data.State = types.StringValue(currentState)

		resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
		return
	}

	app.Config = redactAppConfig(app.Config, currentWriteOnlyMask)
	r.syncAppStateToModel(&data, app, true)
	planned.restoreUpdate(&data)
	data.State = types.StringValue(currentState)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func storeAppWriteOnlyMask(ctx context.Context, privateState interface {
	SetKey(context.Context, string, []byte) diag.Diagnostics
}, writeOnlyValues types.Dynamic) diag.Diagnostics {
	if privateState == nil || (reflect.ValueOf(privateState).Kind() == reflect.Ptr && reflect.ValueOf(privateState).IsNil()) {
		return nil
	}

	mask := dynamicValueToAny(ctx, writeOnlyValues)
	if mask == nil {
		return privateState.SetKey(ctx, appWriteOnlyMaskPrivateStateKey, nil)
	}

	maskJSON, err := json.Marshal(mask)
	if err != nil {
		var diags diag.Diagnostics
		diags.AddError(
			"Unable to Encode Write-Only App Values Mask",
			fmt.Sprintf("Unable to encode write-only values mask: %s", err),
		)
		return diags
	}

	return privateState.SetKey(ctx, appWriteOnlyMaskPrivateStateKey, maskJSON)
}

func loadAppWriteOnlyMask(ctx context.Context, privateState interface {
	GetKey(context.Context, string) ([]byte, diag.Diagnostics)
}) (any, diag.Diagnostics) {
	if privateState == nil || (reflect.ValueOf(privateState).Kind() == reflect.Ptr && reflect.ValueOf(privateState).IsNil()) {
		return nil, nil
	}

	maskJSON, diags := privateState.GetKey(ctx, appWriteOnlyMaskPrivateStateKey)
	if diags.HasError() || len(maskJSON) == 0 {
		return nil, diags
	}

	var mask any
	if err := json.Unmarshal(maskJSON, &mask); err != nil {
		diags.AddError(
			"Unable to Decode Write-Only App Values Mask",
			fmt.Sprintf("Unable to decode write-only values mask: %s", err),
		)
		return nil, diags
	}

	return mask, diags
}

func redactAppConfig(config any, mask any) any {
	redacted, remove := redactAppConfigValue(config, mask)
	if remove {
		return nil
	}
	return redacted
}

func redactAppConfigValue(config any, mask any) (any, bool) {
	if mask == nil {
		return config, false
	}

	switch typedMask := mask.(type) {
	case map[string]any:
		configMap, ok := config.(map[string]any)
		if !ok {
			return config, false
		}

		redacted := make(map[string]any, len(configMap))
		for key, value := range configMap {
			redacted[key] = value
		}

		for key, maskValue := range typedMask {
			configValue, ok := redacted[key]
			if !ok {
				continue
			}

			nested, remove := redactAppConfigValue(configValue, maskValue)
			if remove {
				delete(redacted, key)
				continue
			}
			redacted[key] = nested
		}

		return redacted, false
	case []any:
		configList, ok := config.([]any)
		if !ok {
			return config, false
		}

		redacted := make([]any, len(configList))
		copy(redacted, configList)

		if len(typedMask) == 0 {
			return redacted, false
		}

		if len(typedMask) == 1 {
			for i, configValue := range redacted {
				nested, remove := redactAppConfigValue(configValue, typedMask[0])
				if remove {
					redacted[i] = nil
					continue
				}
				redacted[i] = nested
			}
			return redacted, false
		}

		for i, configValue := range redacted {
			if i >= len(typedMask) {
				break
			}
			nested, remove := redactAppConfigValue(configValue, typedMask[i])
			if remove {
				redacted[i] = nil
				continue
			}
			redacted[i] = nested
		}

		return redacted, false
	default:
		return nil, true
	}
}

func defaultDesiredStateValue(value customtypes.CaseInsensitiveStringValue, fallback string) customtypes.CaseInsensitiveStringValue {
	if value.IsNull() || value.IsUnknown() || value.ValueString() == "" {
		return customtypes.NewCaseInsensitiveStringValue(fallback)
	}
	return value
}

func appStateTimeout(value types.Int64) time.Duration {
	if value.IsNull() || value.IsUnknown() || value.ValueInt64() == 0 {
		return defaultAppStateTimeout
	}
	return time.Duration(value.ValueInt64()) * time.Second
}

func applyCreateDefaults(data *AppResourceModel) {
	if data.Train.IsNull() || data.Train.IsUnknown() || data.Train.ValueString() == "" {
		data.Train = types.StringValue(defaultAppTrain)
	}
	if data.Version.IsNull() || data.Version.IsUnknown() || data.Version.ValueString() == "" {
		data.Version = types.StringValue(defaultAppVersion)
	}
}

func snapshotAppUserState(data AppResourceModel) appUserStateSnapshot {
	return appUserStateSnapshot{
		DesiredState:              data.DesiredState,
		StateTimeout:              data.StateTimeout,
		RestartTriggers:           data.RestartTriggers,
		CatalogApp:                data.CatalogApp,
		Train:                     data.Train,
		Version:                   data.Version,
		Values:                    data.Values,
		WriteOnlyValues:           data.WriteOnlyValues,
		CustomComposeConfig:       data.CustomComposeConfig,
		CustomComposeConfigString: data.CustomComposeConfigString,
		ComposeConfig:             data.ComposeConfig,
	}
}

func (s appUserStateSnapshot) restoreRead(data *AppResourceModel, fallbackDesiredState string) {
	data.DesiredState = defaultDesiredStateValue(s.DesiredState, fallbackDesiredState)
	if s.StateTimeout.IsNull() || s.StateTimeout.IsUnknown() {
		data.StateTimeout = types.Int64Value(int64(defaultAppStateTimeout / time.Second))
	} else {
		data.StateTimeout = s.StateTimeout
	}
	data.RestartTriggers = s.RestartTriggers
	data.CatalogApp = s.CatalogApp
	data.Train = s.Train
	data.Version = s.Version
	if !s.Values.IsNull() && !s.Values.IsUnknown() {
		data.Values = s.Values
	}
}

func (s appUserStateSnapshot) restoreUpdate(data *AppResourceModel) {
	data.DesiredState = defaultDesiredStateValue(s.DesiredState, AppStateRunning)
	if s.StateTimeout.IsNull() || s.StateTimeout.IsUnknown() || s.StateTimeout.ValueInt64() == 0 {
		data.StateTimeout = types.Int64Value(int64(defaultAppStateTimeout / time.Second))
	} else {
		data.StateTimeout = s.StateTimeout
	}
	data.RestartTriggers = s.RestartTriggers
	data.CatalogApp = s.CatalogApp
	data.Train = s.Train
	data.Version = s.Version
	data.Values = s.Values
	data.WriteOnlyValues = types.DynamicNull()
	data.CustomComposeConfig = s.CustomComposeConfig
	data.CustomComposeConfigString = s.CustomComposeConfigString
	data.ComposeConfig = s.ComposeConfig
}

func (r *AppResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data AppResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Call the TrueNAS API
	appName := data.Name.ValueString()
	err := r.services.App.DeleteApp(ctx, appName)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Delete App",
			fmt.Sprintf("Unable to delete app %q: %s", appName, err.Error()),
		)
		return
	}
}

func (r *AppResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// The import ID is the app name - set it to both id and name attributes
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}

// queryAppState queries the TrueNAS API for the current state of an app.
func (r *AppResource) queryAppState(ctx context.Context, name string) (string, error) {
	app, err := r.services.App.GetApp(ctx, name)
	if err != nil {
		return "", err
	}

	if app == nil {
		return "", fmt.Errorf("app %q not found", name)
	}

	return app.State, nil
}

func (r *AppResource) waitForAppStableState(ctx context.Context, name string, timeout time.Duration) (string, error) {
	return waitForStableState(ctx, name, timeout, r.queryAppState)
}

func (r *AppResource) syncAppStateToModel(data *AppResourceModel, app *appEntryResponse, preserveVersionSelector bool) {
	id := app.ID
	if id == "" {
		id = app.Name
	}

	data.ID = types.StringValue(id)
	data.State = types.StringValue(app.State)
	data.CustomApp = types.BoolValue(app.CustomApp)
	data.UpgradeAvailable = types.BoolValue(app.UpgradeAvailable)
	if app.LatestVersion != nil {
		data.LatestVersion = types.StringValue(*app.LatestVersion)
	} else {
		data.LatestVersion = types.StringNull()
	}
	if app.LatestAppVersion != nil {
		data.LatestAppVersion = types.StringValue(*app.LatestAppVersion)
	} else {
		data.LatestAppVersion = types.StringNull()
	}
	data.ImageUpdatesAvailable = types.BoolValue(app.ImageUpdatesAvailable)
	data.Migrated = types.BoolValue(app.Migrated)
	if app.HumanVersion != "" {
		data.HumanVersion = types.StringValue(app.HumanVersion)
	} else {
		data.HumanVersion = types.StringNull()
	}
	if app.Version != "" {
		data.InstalledVersion = types.StringValue(app.Version)
	} else {
		data.InstalledVersion = types.StringNull()
	}
	data.Metadata = anyToDynamicValue(app.Metadata)
	data.ActiveWorkloads = anyToDynamicValue(app.ActiveWorkloads)
	if app.Notes != nil {
		data.Notes = types.StringValue(*app.Notes)
	} else {
		data.Notes = types.StringNull()
	}
	data.Portals = anyToDynamicValue(app.Portals)
	data.VersionDetails = anyToDynamicValue(app.VersionDetails)
	data.Config = anyToDynamicValue(app.Config)

	if app.CustomApp {
		data.Values = types.DynamicNull()
		data.WriteOnlyValues = types.DynamicNull()
		data.CustomComposeConfig = anyToDynamicValue(app.Config)
		if app.Config != nil {
			yamlBytes, err := yaml.Marshal(app.Config)
			if err == nil {
				data.CustomComposeConfigString = customtypes.NewYAMLStringValue(string(yamlBytes))
				data.ComposeConfig = customtypes.NewYAMLStringValue(string(yamlBytes))
			}
		} else {
			data.CustomComposeConfigString = customtypes.NewYAMLStringNull()
			data.ComposeConfig = customtypes.NewYAMLStringNull()
		}
	} else {
		data.Values = anyToDynamicValue(app.Config)
		data.WriteOnlyValues = types.DynamicNull()
		data.CustomComposeConfig = types.DynamicNull()
		data.CustomComposeConfigString = customtypes.NewYAMLStringNull()
		data.ComposeConfig = customtypes.NewYAMLStringNull()
	}

	if !preserveVersionSelector && (data.Version.IsNull() || data.Version.IsUnknown()) {
		data.Version = types.StringValue("latest")
	}
}

// reconcileDesiredState ensures the app is in the desired state.
// It calls StartApp or StopApp as needed and waits for the state to stabilize.
// Returns an error if the reconciliation fails.
func (r *AppResource) reconcileDesiredState(
	ctx context.Context,
	name string,
	currentState string,
	desiredState string,
	timeout time.Duration,
	resp *resource.UpdateResponse,
) error {
	normalizedDesired := normalizeDesiredState(desiredState)

	// Check if reconciliation is needed
	if currentState == normalizedDesired {
		return nil
	}

	// CRASHED is "stopped enough" when desired is STOPPED - no action needed
	if normalizedDesired == AppStateStopped && currentState == AppStateCrashed {
		return nil
	}

	// Add warning about drift
	resp.Diagnostics.AddWarning(
		"App state was externally changed",
		fmt.Sprintf(
			"The app %q was found in state %s but desired_state is %s. "+
				"Reconciling to desired state. To stop this app intentionally, set desired_state = \"stopped\".",
			name, currentState, normalizedDesired,
		),
	)

	// Determine which action to take and call the API
	if normalizedDesired == AppStateRunning {
		if err := r.services.App.StartApp(ctx, name); err != nil {
			return fmt.Errorf("failed to start app %q: %w", name, err)
		}
	} else {
		if err := r.services.App.StopApp(ctx, name); err != nil {
			return fmt.Errorf("failed to stop app %q: %w", name, err)
		}
	}

	finalState, err := r.waitForAppStableState(ctx, name, timeout)
	if err != nil {
		return err
	}

	// Verify we reached the desired state
	if finalState != normalizedDesired {
		return fmt.Errorf("app %q reached state %s instead of desired %s", name, finalState, normalizedDesired)
	}

	return nil
}
