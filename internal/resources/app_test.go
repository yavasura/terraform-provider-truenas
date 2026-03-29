package resources

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/deevus/terraform-provider-truenas/internal/services"
	customtypes "github.com/deevus/terraform-provider-truenas/internal/types"
	truenas "github.com/deevus/truenas-go"
	"github.com/deevus/truenas-go/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestNewAppResource(t *testing.T) {
	r := NewAppResource()
	if r == nil {
		t.Fatal("expected non-nil resource")
	}

	// Verify it implements the required interfaces
	_ = resource.Resource(r)
	var _ resource.ResourceWithConfigure = r.(*AppResource)
	var _ resource.ResourceWithImportState = r.(*AppResource)
}

func TestAppResource_Metadata(t *testing.T) {
	r := NewAppResource()

	req := resource.MetadataRequest{
		ProviderTypeName: "truenas",
	}
	resp := &resource.MetadataResponse{}

	r.Metadata(context.Background(), req, resp)

	if resp.TypeName != "truenas_app" {
		t.Errorf("expected TypeName 'truenas_app', got %q", resp.TypeName)
	}
}

func TestAppResource_Schema(t *testing.T) {
	r := NewAppResource()

	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}

	r.Schema(context.Background(), req, resp)

	// Verify schema has description
	if resp.Schema.Description == "" {
		t.Error("expected non-empty schema description")
	}

	// Verify id attribute exists and is computed
	idAttr, ok := resp.Schema.Attributes["id"]
	if !ok {
		t.Fatal("expected 'id' attribute in schema")
	}
	if !idAttr.IsComputed() {
		t.Error("expected 'id' attribute to be computed")
	}

	// Verify name attribute exists and is required
	nameAttr, ok := resp.Schema.Attributes["name"]
	if !ok {
		t.Fatal("expected 'name' attribute in schema")
	}
	if !nameAttr.IsRequired() {
		t.Error("expected 'name' attribute to be required")
	}

	// Verify custom_app attribute exists and is required
	customAppAttr, ok := resp.Schema.Attributes["custom_app"]
	if !ok {
		t.Fatal("expected 'custom_app' attribute in schema")
	}
	if !customAppAttr.IsRequired() {
		t.Error("expected 'custom_app' attribute to be required")
	}

	// Verify compose_config attribute exists and is optional
	composeConfigAttr, ok := resp.Schema.Attributes["compose_config"]
	if !ok {
		t.Fatal("expected 'compose_config' attribute in schema")
	}
	if !composeConfigAttr.IsOptional() {
		t.Error("expected 'compose_config' attribute to be optional")
	}

	// Verify compose_config uses YAMLStringType for semantic comparison
	stringAttr, ok := composeConfigAttr.(schema.StringAttribute)
	if !ok {
		t.Fatal("expected 'compose_config' to be a StringAttribute")
	}
	if _, ok := stringAttr.CustomType.(customtypes.YAMLStringType); !ok {
		t.Errorf("expected 'compose_config' to use YAMLStringType, got %T", stringAttr.CustomType)
	}

	// Verify state attribute exists and is computed
	stateAttr, ok := resp.Schema.Attributes["state"]
	if !ok {
		t.Fatal("expected 'state' attribute in schema")
	}
	if !stateAttr.IsComputed() {
		t.Error("expected 'state' attribute to be computed")
	}
}

// getAppResourceSchema returns the schema for the app resource
func getAppResourceSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	r := NewAppResource()
	schemaReq := resource.SchemaRequest{}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(context.Background(), schemaReq, schemaResp)
	return *schemaResp
}

// appModelParams contains parameters for creating test app resource model values.
// All fields are optional - nil values result in null tftypes values.
type appModelParams struct {
	ID                        interface{}            // Resource ID (usually same as Name)
	Name                      interface{}            // App name
	CustomApp                 interface{}            // Whether this is a custom app (usually true)
	CatalogApp                interface{}            // Catalog app name
	Train                     interface{}            // Catalog train
	Version                   interface{}            // Desired version selector
	Values                    interface{}            // Values object
	CustomComposeConfig       interface{}            // Structured compose config
	CustomComposeConfigString interface{}            // Compose config string
	ComposeConfig             interface{}            // Deprecated compose config alias
	DesiredState              interface{}            // Desired state: "RUNNING", "STOPPED", "running", "stopped"
	StateTimeout              interface{}            // Timeout in seconds (as float64)
	State                     interface{}            // Actual state from API
	UpgradeAvailable          interface{}            // Whether upgrade is available
	LatestVersion             interface{}            // Latest version from API
	LatestAppVersion          interface{}            // Latest app version from API
	ImageUpdatesAvailable     interface{}            // Whether image updates are available
	Migrated                  interface{}            // Whether app was migrated
	HumanVersion              interface{}            // Human version string
	InstalledVersion          interface{}            // Installed version from API
	Metadata                  interface{}            // Metadata object
	ActiveWorkloads           interface{}            // Active workloads object
	Notes                     interface{}            // Notes string
	Portals                   interface{}            // Portals object
	VersionDetails            interface{}            // Version details object
	Config                    interface{}            // Current API config object
	RestartTriggers           map[string]interface{} // Map of trigger keys to values
}

// newAppModelValue creates a tftypes.Value from appModelParams.
func newAppModelValue(p appModelParams) tftypes.Value {
	// Convert restartTriggers to tftypes.Value
	var triggersValue tftypes.Value
	if p.RestartTriggers == nil {
		triggersValue = tftypes.NewValue(tftypes.Map{ElementType: tftypes.String}, nil)
	} else {
		triggerMap := make(map[string]tftypes.Value)
		for k, v := range p.RestartTriggers {
			triggerMap[k] = tftypes.NewValue(tftypes.String, v)
		}
		triggersValue = tftypes.NewValue(tftypes.Map{ElementType: tftypes.String}, triggerMap)
	}

	dynamicValue := func(v interface{}) tftypes.Value {
		if v == nil {
			return tftypes.NewValue(tftypes.DynamicPseudoType, nil)
		}

		switch val := v.(type) {
		case map[string]interface{}:
			return tftypes.NewValue(tftypes.DynamicPseudoType, toDynamicTFData(val))
		case []interface{}:
			return tftypes.NewValue(tftypes.DynamicPseudoType, toDynamicTFData(val))
		default:
			return tftypes.NewValue(tftypes.DynamicPseudoType, val)
		}
	}

	return tftypes.NewValue(tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"id":                           tftypes.String,
			"name":                         tftypes.String,
			"custom_app":                   tftypes.Bool,
			"catalog_app":                  tftypes.String,
			"train":                        tftypes.String,
			"version":                      tftypes.String,
			"values":                       tftypes.DynamicPseudoType,
			"custom_compose_config":        tftypes.DynamicPseudoType,
			"custom_compose_config_string": tftypes.String,
			"compose_config":               tftypes.String,
			"desired_state":                tftypes.String,
			"state_timeout":                tftypes.Number,
			"state":                        tftypes.String,
			"upgrade_available":            tftypes.Bool,
			"latest_version":               tftypes.String,
			"latest_app_version":           tftypes.String,
			"image_updates_available":      tftypes.Bool,
			"migrated":                     tftypes.Bool,
			"human_version":                tftypes.String,
			"installed_version":            tftypes.String,
			"metadata":                     tftypes.DynamicPseudoType,
			"active_workloads":             tftypes.DynamicPseudoType,
			"notes":                        tftypes.String,
			"portals":                      tftypes.DynamicPseudoType,
			"version_details":              tftypes.DynamicPseudoType,
			"config":                       tftypes.DynamicPseudoType,
			"restart_triggers":             tftypes.Map{ElementType: tftypes.String},
		},
	}, map[string]tftypes.Value{
		"id":                           tftypes.NewValue(tftypes.String, p.ID),
		"name":                         tftypes.NewValue(tftypes.String, p.Name),
		"custom_app":                   tftypes.NewValue(tftypes.Bool, p.CustomApp),
		"catalog_app":                  tftypes.NewValue(tftypes.String, p.CatalogApp),
		"train":                        tftypes.NewValue(tftypes.String, p.Train),
		"version":                      tftypes.NewValue(tftypes.String, p.Version),
		"values":                       dynamicValue(p.Values),
		"custom_compose_config":        dynamicValue(p.CustomComposeConfig),
		"custom_compose_config_string": tftypes.NewValue(tftypes.String, p.CustomComposeConfigString),
		"compose_config":               tftypes.NewValue(tftypes.String, p.ComposeConfig),
		"desired_state":                tftypes.NewValue(tftypes.String, p.DesiredState),
		"state_timeout":                tftypes.NewValue(tftypes.Number, p.StateTimeout),
		"state":                        tftypes.NewValue(tftypes.String, p.State),
		"upgrade_available":            tftypes.NewValue(tftypes.Bool, p.UpgradeAvailable),
		"latest_version":               tftypes.NewValue(tftypes.String, p.LatestVersion),
		"latest_app_version":           tftypes.NewValue(tftypes.String, p.LatestAppVersion),
		"image_updates_available":      tftypes.NewValue(tftypes.Bool, p.ImageUpdatesAvailable),
		"migrated":                     tftypes.NewValue(tftypes.Bool, p.Migrated),
		"human_version":                tftypes.NewValue(tftypes.String, p.HumanVersion),
		"installed_version":            tftypes.NewValue(tftypes.String, p.InstalledVersion),
		"metadata":                     dynamicValue(p.Metadata),
		"active_workloads":             dynamicValue(p.ActiveWorkloads),
		"notes":                        tftypes.NewValue(tftypes.String, p.Notes),
		"portals":                      dynamicValue(p.Portals),
		"version_details":              dynamicValue(p.VersionDetails),
		"config":                       dynamicValue(p.Config),
		"restart_triggers":             triggersValue,
	})
}

func toDynamicTFData(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		attrs := make(map[string]tftypes.Value, len(val))
		for k, item := range val {
			child := toDynamicTFTValue(item)
			attrs[k] = child
		}
		return attrs
	case []interface{}:
		values := make([]tftypes.Value, 0, len(val))
		for _, item := range val {
			child := toDynamicTFTValue(item)
			values = append(values, child)
		}
		return values
	case string:
		return val
	case bool:
		return val
	case int:
		return int64(val)
	case int64:
		return val
	case float64:
		return val
	default:
		return ""
	}
}

func toDynamicTFTValue(v interface{}) tftypes.Value {
	switch val := v.(type) {
	case map[string]interface{}:
		attrs := make(map[string]tftypes.Value, len(val))
		attrTypes := make(map[string]tftypes.Type, len(val))
		for k, item := range val {
			child := toDynamicTFTValue(item)
			attrs[k] = child
			attrTypes[k] = child.Type()
		}
		return tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, attrs)
	case []interface{}:
		if len(val) == 0 {
			return tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{})
		}
		values := make([]tftypes.Value, 0, len(val))
		var elementType tftypes.Type = tftypes.String
		for i, item := range val {
			child := toDynamicTFTValue(item)
			values = append(values, child)
			if i == 0 {
				elementType = child.Type()
			}
		}
		return tftypes.NewValue(tftypes.List{ElementType: elementType}, values)
	case string:
		return tftypes.NewValue(tftypes.String, val)
	case bool:
		return tftypes.NewValue(tftypes.Bool, val)
	case int:
		return tftypes.NewValue(tftypes.Number, int64(val))
	case int64:
		return tftypes.NewValue(tftypes.Number, val)
	case float64:
		return tftypes.NewValue(tftypes.Number, val)
	default:
		return tftypes.NewValue(tftypes.String, "")
	}
}

// createAppResourceModelValue creates a tftypes.Value for the app resource model.
// Deprecated: Use newAppModelValue with appModelParams instead for better readability.
func createAppResourceModelValue(
	id, name interface{},
	customApp interface{},
	composeConfig interface{},
	desiredState interface{},
	stateTimeout interface{},
	state interface{},
) tftypes.Value {
	return newAppModelValue(appModelParams{
		ID:            id,
		Name:          name,
		CustomApp:     customApp,
		ComposeConfig: composeConfig,
		DesiredState:  desiredState,
		StateTimeout:  stateTimeout,
		State:         state,
	})
}

// createAppResourceModelValueWithTriggers creates a tftypes.Value for the app resource model with restart_triggers.
// Deprecated: Use newAppModelValue with appModelParams instead for better readability.
func createAppResourceModelValueWithTriggers(
	id, name interface{},
	customApp interface{},
	composeConfig interface{},
	desiredState interface{},
	stateTimeout interface{},
	state interface{},
	restartTriggers map[string]interface{},
) tftypes.Value {
	return newAppModelValue(appModelParams{
		ID:              id,
		Name:            name,
		CustomApp:       customApp,
		ComposeConfig:   composeConfig,
		DesiredState:    desiredState,
		StateTimeout:    stateTimeout,
		State:           state,
		RestartTriggers: restartTriggers,
	})
}

func TestAppResource_Create_Success(t *testing.T) {
	var capturedOpts truenas.CreateAppOpts

	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				CreateAppFunc: func(ctx context.Context, opts truenas.CreateAppOpts) (*truenas.App, error) {
					capturedOpts = opts
					return &truenas.App{
						Name:  "myapp",
						State: "RUNNING",
					}, nil
				},
			},
		}},
	}

	schemaResp := getAppResourceSchema(t)

	planValue := createAppResourceModelValue(nil, "myapp", true, nil, "RUNNING", float64(120), nil)

	req := resource.CreateRequest{
		Plan: tfsdk.Plan{
			Schema: schemaResp.Schema,
			Raw:    planValue,
		},
	}

	resp := &resource.CreateResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}

	r.Create(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics)
	}

	// Verify create opts
	if capturedOpts.Name != "myapp" {
		t.Errorf("expected Name 'myapp', got %q", capturedOpts.Name)
	}

	if !capturedOpts.CustomApp {
		t.Error("expected CustomApp to be true")
	}

	// Verify state was set
	var model AppResourceModel
	stateDiags := resp.State.Get(context.Background(), &model)
	if stateDiags.HasError() {
		t.Fatalf("failed to get state: %v", stateDiags)
	}

	if model.ID.ValueString() != "myapp" {
		t.Errorf("expected ID 'myapp', got %q", model.ID.ValueString())
	}

	if model.State.ValueString() != "RUNNING" {
		t.Errorf("expected State 'RUNNING', got %q", model.State.ValueString())
	}
}

func TestAppResource_Create_WithComposeConfig(t *testing.T) {
	var capturedOpts truenas.CreateAppOpts

	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				CreateAppFunc: func(ctx context.Context, opts truenas.CreateAppOpts) (*truenas.App, error) {
					capturedOpts = opts
					return &truenas.App{
						Name:  "myapp",
						State: "RUNNING",
					}, nil
				},
			},
		}},
	}

	schemaResp := getAppResourceSchema(t)

	composeYAML := "version: '3'\nservices:\n  web:\n    image: nginx"
	planValue := createAppResourceModelValue(nil, "myapp", true, composeYAML, "RUNNING", float64(120), nil)

	req := resource.CreateRequest{
		Plan: tfsdk.Plan{
			Schema: schemaResp.Schema,
			Raw:    planValue,
		},
	}

	resp := &resource.CreateResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}

	r.Create(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics)
	}

	// Verify opts include compose config
	if capturedOpts.CustomComposeConfig != composeYAML {
		t.Errorf("expected compose config %q, got %q", composeYAML, capturedOpts.CustomComposeConfig)
	}
}

func TestAppResource_Create_APIError(t *testing.T) {
	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				CreateAppFunc: func(ctx context.Context, opts truenas.CreateAppOpts) (*truenas.App, error) {
					return nil, errors.New("app already exists")
				},
			},
		}},
	}

	schemaResp := getAppResourceSchema(t)

	planValue := createAppResourceModelValue(nil, "myapp", true, nil, "RUNNING", float64(120), nil)

	req := resource.CreateRequest{
		Plan: tfsdk.Plan{
			Schema: schemaResp.Schema,
			Raw:    planValue,
		},
	}

	resp := &resource.CreateResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}

	r.Create(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for API error")
	}
}

func TestAppResource_Read_Success(t *testing.T) {
	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				GetAppWithConfigFunc: func(ctx context.Context, name string) (*truenas.App, error) {
					return &truenas.App{
						Name:      "myapp",
						State:     "RUNNING",
						CustomApp: true,
						Config: map[string]any{
							"services": map[string]any{
								"web": map[string]any{
									"image": "nginx",
								},
							},
						},
					}, nil
				},
			},
		}},
	}

	schemaResp := getAppResourceSchema(t)

	stateValue := createAppResourceModelValue("myapp", "myapp", true, nil, "RUNNING", float64(120), "STOPPED")

	req := resource.ReadRequest{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    stateValue,
		},
	}

	resp := &resource.ReadResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}

	r.Read(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics)
	}

	// Verify state was updated
	var model AppResourceModel
	diags := resp.State.Get(context.Background(), &model)
	if diags.HasError() {
		t.Fatalf("failed to get state: %v", diags)
	}

	if model.ID.ValueString() != "myapp" {
		t.Errorf("expected ID 'myapp', got %q", model.ID.ValueString())
	}

	if model.State.ValueString() != "RUNNING" {
		t.Errorf("expected State 'RUNNING', got %q", model.State.ValueString())
	}

	if !model.CustomApp.ValueBool() {
		t.Error("expected CustomApp to be true")
	}

	// Config is returned as parsed YAML, then marshaled back - verify it contains expected content
	composeConfig := model.ComposeConfig.ValueString()
	if composeConfig == "" {
		t.Error("expected compose_config to be synced, got empty string")
	}
	if !strings.Contains(composeConfig, "services:") || !strings.Contains(composeConfig, "image: nginx") {
		t.Errorf("expected compose_config to contain services and nginx image, got %q", composeConfig)
	}
}

func TestAppResource_Read_NotFound(t *testing.T) {
	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				GetAppWithConfigFunc: func(ctx context.Context, name string) (*truenas.App, error) {
					return nil, nil // not found
				},
			},
		}},
	}

	schemaResp := getAppResourceSchema(t)

	stateValue := createAppResourceModelValue("myapp", "myapp", true, nil, "RUNNING", float64(120), "RUNNING")

	req := resource.ReadRequest{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    stateValue,
		},
	}

	resp := &resource.ReadResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}

	r.Read(context.Background(), req, resp)

	// Should NOT have errors - just remove from state
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics)
	}

	// State should be empty (removed)
	if !resp.State.Raw.IsNull() {
		t.Error("expected state to be removed (null) when app not found")
	}
}

func TestAppResource_Read_APIError(t *testing.T) {
	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				GetAppWithConfigFunc: func(ctx context.Context, name string) (*truenas.App, error) {
					return nil, errors.New("connection failed")
				},
			},
		}},
	}

	schemaResp := getAppResourceSchema(t)

	stateValue := createAppResourceModelValue("myapp", "myapp", true, nil, "RUNNING", float64(120), "RUNNING")

	req := resource.ReadRequest{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    stateValue,
		},
	}

	resp := &resource.ReadResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}

	r.Read(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for API error")
	}
}

func TestAppResource_Update_Success(t *testing.T) {
	var capturedUpdateName string
	var capturedUpdateOpts truenas.UpdateAppOpts
	var getAppCalled bool

	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				UpdateAppFunc: func(ctx context.Context, name string, opts truenas.UpdateAppOpts) (*truenas.App, error) {
					capturedUpdateName = name
					capturedUpdateOpts = opts
					return &truenas.App{
						Name:  "myapp",
						State: "RUNNING",
					}, nil
				},
				GetAppFunc: func(ctx context.Context, name string) (*truenas.App, error) {
					getAppCalled = true
					return &truenas.App{
						Name:  "myapp",
						State: "RUNNING",
					}, nil
				},
			},
		}},
	}

	schemaResp := getAppResourceSchema(t)

	// Current state
	stateValue := createAppResourceModelValue("myapp", "myapp", true, nil, "RUNNING", float64(120), "STOPPED")

	// Plan with new compose config
	composeYAML := "version: '3'\nservices:\n  web:\n    image: nginx:latest"
	planValue := createAppResourceModelValue("myapp", "myapp", true, composeYAML, "RUNNING", float64(120), nil)

	req := resource.UpdateRequest{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    stateValue,
		},
		Plan: tfsdk.Plan{
			Schema: schemaResp.Schema,
			Raw:    planValue,
		},
	}

	resp := &resource.UpdateResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}

	r.Update(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics)
	}

	// Verify UpdateApp was called
	if capturedUpdateName != "myapp" {
		t.Errorf("expected update name 'myapp', got %q", capturedUpdateName)
	}

	// Verify update opts contain compose config
	if capturedUpdateOpts.CustomComposeConfig != composeYAML {
		t.Errorf("expected compose config %q, got %q", composeYAML, capturedUpdateOpts.CustomComposeConfig)
	}

	// Verify GetApp was called to get state after update
	if !getAppCalled {
		t.Error("expected GetApp to be called to query state after update")
	}
}

func TestAppResource_Update_APIError(t *testing.T) {
	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				UpdateAppFunc: func(ctx context.Context, name string, opts truenas.UpdateAppOpts) (*truenas.App, error) {
					return nil, errors.New("update failed")
				},
			},
		}},
	}

	schemaResp := getAppResourceSchema(t)

	stateValue := createAppResourceModelValue("myapp", "myapp", true, nil, "RUNNING", float64(120), "STOPPED")
	planValue := createAppResourceModelValue("myapp", "myapp", true, "new: config", "RUNNING", float64(120), nil)

	req := resource.UpdateRequest{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    stateValue,
		},
		Plan: tfsdk.Plan{
			Schema: schemaResp.Schema,
			Raw:    planValue,
		},
	}

	resp := &resource.UpdateResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}

	r.Update(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for API error")
	}
}

func TestAppResource_Delete_Success(t *testing.T) {
	var capturedName string

	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				DeleteAppFunc: func(ctx context.Context, name string) error {
					capturedName = name
					return nil
				},
			},
		}},
	}

	schemaResp := getAppResourceSchema(t)

	stateValue := createAppResourceModelValue("myapp", "myapp", true, nil, "RUNNING", float64(120), "RUNNING")

	req := resource.DeleteRequest{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    stateValue,
		},
	}

	resp := &resource.DeleteResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}

	r.Delete(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics)
	}

	// Verify app.delete was called
	if capturedName != "myapp" {
		t.Errorf("expected name 'myapp', got %q", capturedName)
	}
}

func TestAppResource_Delete_APIError(t *testing.T) {
	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				DeleteAppFunc: func(ctx context.Context, name string) error {
					return errors.New("app is running")
				},
			},
		}},
	}

	schemaResp := getAppResourceSchema(t)

	stateValue := createAppResourceModelValue("myapp", "myapp", true, nil, "RUNNING", float64(120), "RUNNING")

	req := resource.DeleteRequest{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    stateValue,
		},
	}

	resp := &resource.DeleteResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}

	r.Delete(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for API error")
	}
}

func TestAppResource_ImportState(t *testing.T) {
	r := NewAppResource().(*AppResource)

	schemaResp := getAppResourceSchema(t)

	// Create an initial empty state with the correct schema
	emptyState := createAppResourceModelValue(nil, nil, nil, nil, nil, nil, nil)

	req := resource.ImportStateRequest{
		ID: "imported-app",
	}

	resp := &resource.ImportStateResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    emptyState,
		},
	}

	r.ImportState(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics)
	}

	// Verify state has id set to the import ID
	var model AppResourceModel
	diags := resp.State.Get(context.Background(), &model)
	if diags.HasError() {
		t.Fatalf("failed to get state: %v", diags)
	}

	if model.ID.ValueString() != "imported-app" {
		t.Errorf("expected ID 'imported-app', got %q", model.ID.ValueString())
	}

	if model.Name.ValueString() != "imported-app" {
		t.Errorf("expected Name 'imported-app', got %q", model.Name.ValueString())
	}
}

// Test interface compliance
func TestAppResource_ImplementsInterfaces(t *testing.T) {
	r := NewAppResource()

	_ = resource.Resource(r)
	_ = resource.ResourceWithConfigure(r.(*AppResource))
	_ = resource.ResourceWithImportState(r.(*AppResource))
}

// Test Read with empty compose_config sets null
func TestAppResource_Read_EmptyComposeConfigSetsNull(t *testing.T) {
	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				GetAppWithConfigFunc: func(ctx context.Context, name string) (*truenas.App, error) {
					return &truenas.App{
						Name:      "myapp",
						State:     "RUNNING",
						CustomApp: false,
						Config:    map[string]any{},
					}, nil
				},
			},
		}},
	}

	schemaResp := getAppResourceSchema(t)

	stateValue := createAppResourceModelValue("myapp", "myapp", true, "old config", "RUNNING", float64(120), "STOPPED")

	req := resource.ReadRequest{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    stateValue,
		},
	}

	resp := &resource.ReadResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}

	r.Read(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics)
	}

	var model AppResourceModel
	stateDiags := resp.State.Get(context.Background(), &model)
	if stateDiags.HasError() {
		t.Fatalf("failed to get state: %v", stateDiags)
	}

	// compose_config should be null when API returns empty config
	if !model.ComposeConfig.IsNull() {
		t.Errorf("expected compose_config to be null, got %q", model.ComposeConfig.ValueString())
	}

	// custom_app should be synced from API
	if model.CustomApp.ValueBool() {
		t.Error("expected CustomApp to be false (synced from API)")
	}
}

// Test Update with query error after update (GetApp error)
func TestAppResource_Update_QueryErrorAfterUpdate(t *testing.T) {
	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				UpdateAppFunc: func(ctx context.Context, name string, opts truenas.UpdateAppOpts) (*truenas.App, error) {
					return &truenas.App{Name: "myapp", State: "RUNNING"}, nil
				},
				GetAppFunc: func(ctx context.Context, name string) (*truenas.App, error) {
					return nil, errors.New("query failed")
				},
			},
		}},
	}

	schemaResp := getAppResourceSchema(t)

	stateValue := createAppResourceModelValue("myapp", "myapp", true, nil, "RUNNING", float64(120), "STOPPED")
	planValue := createAppResourceModelValue("myapp", "myapp", true, "new: config", "RUNNING", float64(120), nil)

	req := resource.UpdateRequest{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    stateValue,
		},
		Plan: tfsdk.Plan{
			Schema: schemaResp.Schema,
			Raw:    planValue,
		},
	}

	resp := &resource.UpdateResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}

	r.Update(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when query fails after update")
	}
}

// Test Update when app is not found after update (GetApp returns nil)
func TestAppResource_Update_AppNotFoundAfterUpdate(t *testing.T) {
	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				UpdateAppFunc: func(ctx context.Context, name string, opts truenas.UpdateAppOpts) (*truenas.App, error) {
					return &truenas.App{Name: "myapp", State: "RUNNING"}, nil
				},
				GetAppFunc: func(ctx context.Context, name string) (*truenas.App, error) {
					return nil, nil // not found
				},
			},
		}},
	}

	schemaResp := getAppResourceSchema(t)

	stateValue := createAppResourceModelValue("myapp", "myapp", true, nil, "RUNNING", float64(120), "STOPPED")
	planValue := createAppResourceModelValue("myapp", "myapp", true, "new: config", "RUNNING", float64(120), nil)

	req := resource.UpdateRequest{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    stateValue,
		},
		Plan: tfsdk.Plan{
			Schema: schemaResp.Schema,
			Raw:    planValue,
		},
	}

	resp := &resource.UpdateResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}

	r.Update(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when app not found after update")
	}
}

func TestAppResource_Schema_DesiredStateAttribute(t *testing.T) {
	r := NewAppResource()

	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}

	r.Schema(context.Background(), req, resp)

	// Verify desired_state attribute exists and is optional
	desiredStateAttr, ok := resp.Schema.Attributes["desired_state"]
	if !ok {
		t.Fatal("expected 'desired_state' attribute in schema")
	}
	if !desiredStateAttr.IsOptional() {
		t.Error("expected 'desired_state' attribute to be optional")
	}

	// Verify state_timeout attribute exists and is optional
	stateTimeoutAttr, ok := resp.Schema.Attributes["state_timeout"]
	if !ok {
		t.Fatal("expected 'state_timeout' attribute in schema")
	}
	if !stateTimeoutAttr.IsOptional() {
		t.Error("expected 'state_timeout' attribute to be optional")
	}
}

// Test ImportState followed by Read verifies the flow works
func TestAppResource_queryAppState_Success(t *testing.T) {
	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				GetAppFunc: func(ctx context.Context, name string) (*truenas.App, error) {
					return &truenas.App{Name: "myapp", State: "RUNNING"}, nil
				},
			},
		}},
	}

	state, err := r.queryAppState(context.Background(), "myapp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != "RUNNING" {
		t.Errorf("expected state RUNNING, got %q", state)
	}
}

func TestAppResource_queryAppState_NotFound(t *testing.T) {
	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				GetAppFunc: func(ctx context.Context, name string) (*truenas.App, error) {
					return nil, nil // not found
				},
			},
		}},
	}

	_, err := r.queryAppState(context.Background(), "myapp")
	if err == nil {
		t.Fatal("expected error for app not found")
	}
}

func TestAppResource_queryAppState_APIError(t *testing.T) {
	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				GetAppFunc: func(ctx context.Context, name string) (*truenas.App, error) {
					return nil, errors.New("connection failed")
				},
			},
		}},
	}

	_, err := r.queryAppState(context.Background(), "myapp")
	if err == nil {
		t.Fatal("expected error for API error")
	}
}

func TestAppResource_ImportState_FollowedByRead(t *testing.T) {
	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				GetAppWithConfigFunc: func(ctx context.Context, name string) (*truenas.App, error) {
					return &truenas.App{
						Name:      "imported-app",
						State:     "RUNNING",
						CustomApp: true,
						Config: map[string]any{
							"version": "3",
						},
					}, nil
				},
			},
		}},
	}

	schemaResp := getAppResourceSchema(t)

	// Step 1: Import state
	emptyState := createAppResourceModelValue(nil, nil, nil, nil, nil, nil, nil)

	importReq := resource.ImportStateRequest{
		ID: "imported-app",
	}

	importResp := &resource.ImportStateResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    emptyState,
		},
	}

	r.ImportState(context.Background(), importReq, importResp)

	if importResp.Diagnostics.HasError() {
		t.Fatalf("import state errors: %v", importResp.Diagnostics)
	}

	// Step 2: Read to refresh state from API
	readReq := resource.ReadRequest{
		State: importResp.State,
	}

	readResp := &resource.ReadResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}

	r.Read(context.Background(), readReq, readResp)

	if readResp.Diagnostics.HasError() {
		t.Fatalf("read errors: %v", readResp.Diagnostics)
	}

	// Verify all fields were populated from API
	var model AppResourceModel
	diags := readResp.State.Get(context.Background(), &model)
	if diags.HasError() {
		t.Fatalf("failed to get state: %v", diags)
	}

	if model.ID.ValueString() != "imported-app" {
		t.Errorf("expected ID 'imported-app', got %q", model.ID.ValueString())
	}

	if model.Name.ValueString() != "imported-app" {
		t.Errorf("expected Name 'imported-app', got %q", model.Name.ValueString())
	}

	if !model.CustomApp.ValueBool() {
		t.Error("expected CustomApp to be true (populated from API)")
	}

	if model.State.ValueString() != "RUNNING" {
		t.Errorf("expected State 'RUNNING', got %q", model.State.ValueString())
	}

	// Config is returned as parsed YAML, then marshaled back
	composeConfig := model.ComposeConfig.ValueString()
	if !strings.Contains(composeConfig, "version:") || !strings.Contains(composeConfig, "\"3\"") {
		t.Errorf("expected compose_config to contain version: 3, got %q", composeConfig)
	}
}

func TestAppResource_reconcileDesiredState_StartApp(t *testing.T) {
	var calledStart bool
	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				StartAppFunc: func(ctx context.Context, name string) error {
					calledStart = true
					return nil
				},
				GetAppFunc: func(ctx context.Context, name string) (*truenas.App, error) {
					return &truenas.App{Name: "myapp", State: "RUNNING"}, nil
				},
			},
		}},
	}

	schemaResp := getAppResourceSchema(t)
	resp := &resource.UpdateResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}
	err := r.reconcileDesiredState(context.Background(), "myapp", "STOPPED", "RUNNING", 30*time.Second, resp)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !calledStart {
		t.Error("expected StartApp to be called")
	}
}

func TestAppResource_reconcileDesiredState_StopApp(t *testing.T) {
	var calledStop bool
	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				StopAppFunc: func(ctx context.Context, name string) error {
					calledStop = true
					return nil
				},
				GetAppFunc: func(ctx context.Context, name string) (*truenas.App, error) {
					return &truenas.App{Name: "myapp", State: "STOPPED"}, nil
				},
			},
		}},
	}

	schemaResp := getAppResourceSchema(t)
	resp := &resource.UpdateResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}
	err := r.reconcileDesiredState(context.Background(), "myapp", "RUNNING", "STOPPED", 30*time.Second, resp)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !calledStop {
		t.Error("expected StopApp to be called")
	}
}

func TestAppResource_reconcileDesiredState_NoChangeNeeded(t *testing.T) {
	startCalled := false
	stopCalled := false
	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				StartAppFunc: func(ctx context.Context, name string) error {
					startCalled = true
					return nil
				},
				StopAppFunc: func(ctx context.Context, name string) error {
					stopCalled = true
					return nil
				},
			},
		}},
	}

	schemaResp := getAppResourceSchema(t)
	resp := &resource.UpdateResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}
	err := r.reconcileDesiredState(context.Background(), "myapp", "RUNNING", "RUNNING", 30*time.Second, resp)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if startCalled || stopCalled {
		t.Error("expected no API calls when state matches")
	}
}

func TestAppResource_Update_ReconcileStateFromStoppedToRunning(t *testing.T) {
	var startCalled bool
	queryCount := 0
	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				StartAppFunc: func(ctx context.Context, name string) error {
					startCalled = true
					return nil
				},
				GetAppFunc: func(ctx context.Context, name string) (*truenas.App, error) {
					queryCount++
					// First query: return STOPPED (simulating external change)
					// Second query (after start): return RUNNING
					if queryCount == 1 {
						return &truenas.App{Name: "myapp", State: "STOPPED"}, nil
					}
					return &truenas.App{Name: "myapp", State: "RUNNING"}, nil
				},
			},
		}},
	}

	schemaResp := getAppResourceSchema(t)

	// Current state: STOPPED, desired: RUNNING
	stateValue := createAppResourceModelValue("myapp", "myapp", true, nil, "RUNNING", float64(120), "STOPPED")
	planValue := createAppResourceModelValue("myapp", "myapp", true, nil, "RUNNING", float64(120), nil)

	req := resource.UpdateRequest{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    stateValue,
		},
		Plan: tfsdk.Plan{
			Schema: schemaResp.Schema,
			Raw:    planValue,
		},
	}

	resp := &resource.UpdateResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}

	r.Update(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics)
	}

	// Verify StartApp was called
	if !startCalled {
		t.Error("expected StartApp to be called")
	}

	// Verify warning was added about drift
	hasWarning := false
	for _, d := range resp.Diagnostics.Warnings() {
		if strings.Contains(d.Summary(), "externally changed") {
			hasWarning = true
			break
		}
	}
	if !hasWarning {
		t.Error("expected drift warning to be added")
	}
}

func TestAppResource_Read_PreservesDesiredState(t *testing.T) {
	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				GetAppWithConfigFunc: func(ctx context.Context, name string) (*truenas.App, error) {
					return &truenas.App{
						Name:      "myapp",
						State:     "RUNNING",
						CustomApp: true,
						Config:    map[string]any{},
					}, nil
				},
			},
		}},
	}

	schemaResp := getAppResourceSchema(t)

	// Prior state has desired_state = "STOPPED" (user intentionally wants it stopped)
	// but API returns state = "RUNNING" (maybe it was started externally)
	stateValue := createAppResourceModelValue("myapp", "myapp", true, nil, "STOPPED", float64(180), "STOPPED")

	req := resource.ReadRequest{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    stateValue,
		},
	}

	resp := &resource.ReadResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}

	r.Read(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics)
	}

	var model AppResourceModel
	diags := resp.State.Get(context.Background(), &model)
	if diags.HasError() {
		t.Fatalf("failed to get state: %v", diags)
	}

	// desired_state should be preserved from prior state (not reset to current state)
	if model.DesiredState.ValueString() != "STOPPED" {
		t.Errorf("expected desired_state 'STOPPED' to be preserved, got %q", model.DesiredState.ValueString())
	}

	// state_timeout should be preserved from prior state
	if model.StateTimeout.ValueInt64() != 180 {
		t.Errorf("expected state_timeout 180 to be preserved, got %d", model.StateTimeout.ValueInt64())
	}

	// state should reflect actual API state
	if model.State.ValueString() != "RUNNING" {
		t.Errorf("expected state 'RUNNING' from API, got %q", model.State.ValueString())
	}
}

// TestAppResource_Read_PreservesDesiredStateCase verifies that Read preserves
// the user's original case for desired_state (bug fix for "planned value does
// not match config value" errors).
func TestAppResource_Read_PreservesDesiredStateCase(t *testing.T) {
	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				GetAppWithConfigFunc: func(ctx context.Context, name string) (*truenas.App, error) {
					return &truenas.App{
						Name:      "myapp",
						State:     "STOPPED",
						CustomApp: true,
						Config:    map[string]any{},
					}, nil
				},
			},
		}},
	}

	schemaResp := getAppResourceSchema(t)

	// Prior state has lowercase desired_state = "stopped"
	stateValue := newAppModelValue(appModelParams{
		ID:           "myapp",
		Name:         "myapp",
		CustomApp:    true,
		DesiredState: "stopped",
		StateTimeout: float64(120),
		State:        "STOPPED",
	})

	req := resource.ReadRequest{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    stateValue,
		},
	}

	resp := &resource.ReadResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}

	r.Read(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics)
	}

	var model AppResourceModel
	diags := resp.State.Get(context.Background(), &model)
	if diags.HasError() {
		t.Fatalf("failed to get state: %v", diags)
	}

	// Critical: desired_state should preserve user's lowercase "stopped"
	if model.DesiredState.ValueString() != "stopped" {
		t.Errorf("expected lowercase 'stopped' to be preserved, got %q", model.DesiredState.ValueString())
	}
}

func TestAppResource_Read_DefaultsDesiredStateWhenNull(t *testing.T) {
	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				GetAppWithConfigFunc: func(ctx context.Context, name string) (*truenas.App, error) {
					return &truenas.App{
						Name:      "myapp",
						State:     "RUNNING",
						CustomApp: true,
						Config:    map[string]any{},
					}, nil
				},
			},
		}},
	}

	schemaResp := getAppResourceSchema(t)

	// Prior state has null desired_state (like after import)
	stateValue := createAppResourceModelValue("myapp", "myapp", true, nil, nil, nil, nil)

	req := resource.ReadRequest{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    stateValue,
		},
	}

	resp := &resource.ReadResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}

	r.Read(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics)
	}

	var model AppResourceModel
	diags := resp.State.Get(context.Background(), &model)
	if diags.HasError() {
		t.Fatalf("failed to get state: %v", diags)
	}

	// When desired_state is null, it should default to current state from API
	if model.DesiredState.ValueString() != "RUNNING" {
		t.Errorf("expected desired_state to default to 'RUNNING', got %q", model.DesiredState.ValueString())
	}

	// When state_timeout is null, it should default to 120
	if model.StateTimeout.ValueInt64() != 120 {
		t.Errorf("expected state_timeout to default to 120, got %d", model.StateTimeout.ValueInt64())
	}
}

func TestAppResource_Create_WithDesiredStateStopped(t *testing.T) {
	var createCalled bool
	var stopCalled bool
	queryCount := 0
	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				CreateAppFunc: func(ctx context.Context, opts truenas.CreateAppOpts) (*truenas.App, error) {
					createCalled = true
					return &truenas.App{Name: "myapp", State: "RUNNING"}, nil
				},
				StopAppFunc: func(ctx context.Context, name string) error {
					stopCalled = true
					return nil
				},
				GetAppFunc: func(ctx context.Context, name string) (*truenas.App, error) {
					queryCount++
					// After stop is called, return STOPPED
					if stopCalled {
						return &truenas.App{Name: "myapp", State: "STOPPED"}, nil
					}
					return &truenas.App{Name: "myapp", State: "RUNNING"}, nil
				},
			},
		}},
	}

	schemaResp := getAppResourceSchema(t)

	// Plan with desired_state = "stopped"
	planValue := createAppResourceModelValue(nil, "myapp", true, nil, "stopped", float64(120), nil)

	req := resource.CreateRequest{
		Plan: tfsdk.Plan{
			Schema: schemaResp.Schema,
			Raw:    planValue,
		},
	}

	resp := &resource.CreateResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}

	r.Create(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics)
	}

	// Verify CreateApp was called, then StopApp
	if !createCalled {
		t.Error("expected CreateApp to be called")
	}
	if !stopCalled {
		t.Error("expected StopApp to be called")
	}

	// Verify final state
	var model AppResourceModel
	diags := resp.State.Get(context.Background(), &model)
	if diags.HasError() {
		t.Fatalf("failed to get state: %v", diags)
	}
	if model.State.ValueString() != "STOPPED" {
		t.Errorf("expected final state STOPPED, got %q", model.State.ValueString())
	}
	// Verify desired_state preserves user's original case (bug fix: lowercase should stay lowercase)
	if model.DesiredState.ValueString() != "stopped" {
		t.Errorf("expected desired_state to preserve user's lowercase 'stopped', got %q", model.DesiredState.ValueString())
	}
}

// TestAppResource_Create_DesiredStateCasePreservation tests that user-specified
// desired_state values are preserved exactly as written (case-insensitive comparison
// but case-preserving storage). This prevents Terraform "planned value does not match
// config value" errors when users specify lowercase values like "stopped".
func TestAppResource_Create_DesiredStateCasePreservation(t *testing.T) {
	tests := []struct {
		name          string
		inputDesired  string
		expectedState string // API always returns uppercase
	}{
		{
			name:          "lowercase stopped",
			inputDesired:  "stopped",
			expectedState: "STOPPED",
		},
		{
			name:          "uppercase STOPPED",
			inputDesired:  "STOPPED",
			expectedState: "STOPPED",
		},
		{
			name:          "lowercase running",
			inputDesired:  "running",
			expectedState: "RUNNING",
		},
		{
			name:          "mixed case Running",
			inputDesired:  "Running",
			expectedState: "RUNNING",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := &AppResource{
				BaseResource: BaseResource{services: &services.TrueNASServices{
					App: &truenas.MockAppService{
						CreateAppFunc: func(ctx context.Context, opts truenas.CreateAppOpts) (*truenas.App, error) {
							return &truenas.App{Name: "myapp", State: tc.expectedState}, nil
						},
						StartAppFunc: func(ctx context.Context, name string) error {
							return nil
						},
						StopAppFunc: func(ctx context.Context, name string) error {
							return nil
						},
						GetAppFunc: func(ctx context.Context, name string) (*truenas.App, error) {
							return &truenas.App{Name: "myapp", State: tc.expectedState}, nil
						},
					},
				}},
			}

			schemaResp := getAppResourceSchema(t)
			planValue := newAppModelValue(appModelParams{
				Name:         "myapp",
				CustomApp:    true,
				DesiredState: tc.inputDesired,
				StateTimeout: float64(120),
			})

			req := resource.CreateRequest{
				Plan: tfsdk.Plan{
					Schema: schemaResp.Schema,
					Raw:    planValue,
				},
			}

			resp := &resource.CreateResponse{
				State: tfsdk.State{
					Schema: schemaResp.Schema,
				},
			}

			r.Create(context.Background(), req, resp)

			if resp.Diagnostics.HasError() {
				t.Fatalf("unexpected errors: %v", resp.Diagnostics)
			}

			var model AppResourceModel
			diags := resp.State.Get(context.Background(), &model)
			if diags.HasError() {
				t.Fatalf("failed to get state: %v", diags)
			}

			// Key assertion: desired_state should preserve user's original case
			// This is critical to prevent "planned value does not match config value" errors
			if model.DesiredState.ValueString() != tc.inputDesired {
				t.Errorf("desired_state was not preserved: expected %q, got %q", tc.inputDesired, model.DesiredState.ValueString())
			}

			// State should reflect the API's response (always uppercase)
			if model.State.ValueString() != tc.expectedState {
				t.Errorf("state mismatch: expected %q, got %q", tc.expectedState, model.State.ValueString())
			}
		})
	}
}

func TestAppResource_Update_CrashedAppStartAttempt(t *testing.T) {
	var startCalled bool
	queryCount := 0
	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				StartAppFunc: func(ctx context.Context, name string) error {
					startCalled = true
					return nil
				},
				GetAppFunc: func(ctx context.Context, name string) (*truenas.App, error) {
					queryCount++
					// First query: return CRASHED (the current state)
					// Subsequent queries: return RUNNING (after start attempt)
					if queryCount == 1 {
						return &truenas.App{Name: "myapp", State: "CRASHED"}, nil
					}
					return &truenas.App{Name: "myapp", State: "RUNNING"}, nil
				},
			},
		}},
	}

	schemaResp := getAppResourceSchema(t)

	// Current state: CRASHED, desired: RUNNING
	stateValue := createAppResourceModelValue("myapp", "myapp", true, nil, "RUNNING", float64(120), "CRASHED")
	planValue := createAppResourceModelValue("myapp", "myapp", true, nil, "RUNNING", float64(120), nil)

	req := resource.UpdateRequest{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    stateValue,
		},
		Plan: tfsdk.Plan{
			Schema: schemaResp.Schema,
			Raw:    planValue,
		},
	}

	resp := &resource.UpdateResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}

	r.Update(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics)
	}

	// Verify StartApp was called to recover from CRASHED
	if !startCalled {
		t.Error("expected StartApp to be called for CRASHED app")
	}
}

func TestAppResource_Update_CrashedAppDesiredStopped(t *testing.T) {
	startCalled := false
	stopCalled := false
	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				StartAppFunc: func(ctx context.Context, name string) error {
					startCalled = true
					return nil
				},
				StopAppFunc: func(ctx context.Context, name string) error {
					stopCalled = true
					return nil
				},
				GetAppFunc: func(ctx context.Context, name string) (*truenas.App, error) {
					return &truenas.App{Name: "myapp", State: "CRASHED"}, nil
				},
			},
		}},
	}

	schemaResp := getAppResourceSchema(t)

	// Current state: CRASHED, desired: STOPPED - no action needed
	stateValue := createAppResourceModelValue("myapp", "myapp", true, nil, "STOPPED", float64(120), "CRASHED")
	planValue := createAppResourceModelValue("myapp", "myapp", true, nil, "STOPPED", float64(120), nil)

	req := resource.UpdateRequest{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    stateValue,
		},
		Plan: tfsdk.Plan{
			Schema: schemaResp.Schema,
			Raw:    planValue,
		},
	}

	resp := &resource.UpdateResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}

	r.Update(context.Background(), req, resp)

	// Should not error - CRASHED is "stopped enough"
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics)
	}

	// No start/stop should be called
	if startCalled || stopCalled {
		t.Error("expected no API calls for CRASHED->STOPPED")
	}
}

func TestAppResource_Schema_RestartTriggersAttribute(t *testing.T) {
	r := NewAppResource()

	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}

	r.Schema(context.Background(), req, resp)

	// Verify restart_triggers attribute exists and is optional
	restartTriggersAttr, ok := resp.Schema.Attributes["restart_triggers"]
	if !ok {
		t.Fatal("expected 'restart_triggers' attribute in schema")
	}
	if !restartTriggersAttr.IsOptional() {
		t.Error("expected 'restart_triggers' attribute to be optional")
	}
}

func TestAppResource_Update_RestartTriggersChange(t *testing.T) {
	var stopCalled, startCalled bool
	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				StopAppFunc: func(ctx context.Context, name string) error {
					stopCalled = true
					return nil
				},
				StartAppFunc: func(ctx context.Context, name string) error {
					startCalled = true
					return nil
				},
				GetAppFunc: func(ctx context.Context, name string) (*truenas.App, error) {
					// Return RUNNING state throughout - app should be restarted
					return &truenas.App{Name: "myapp", State: "RUNNING"}, nil
				},
			},
		}},
	}

	schemaResp := getAppResourceSchema(t)

	// Current state: has restart_triggers with old checksum
	stateValue := createAppResourceModelValueWithTriggers(
		"myapp", "myapp", true, nil, "RUNNING", float64(120), "RUNNING",
		map[string]interface{}{"config_checksum": "old_checksum"},
	)

	// Plan: has restart_triggers with new checksum (file changed)
	planValue := createAppResourceModelValueWithTriggers(
		"myapp", "myapp", true, nil, "RUNNING", float64(120), nil,
		map[string]interface{}{"config_checksum": "new_checksum"},
	)

	req := resource.UpdateRequest{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    stateValue,
		},
		Plan: tfsdk.Plan{
			Schema: schemaResp.Schema,
			Raw:    planValue,
		},
	}

	resp := &resource.UpdateResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}

	r.Update(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics)
	}

	// Verify app.stop then app.start were called (restart)
	if !stopCalled {
		t.Error("expected StopApp to be called for restart")
	}
	if !startCalled {
		t.Error("expected StartApp to be called for restart")
	}
}

func TestAppResource_Update_RestartTriggersNoChangeNoRestart(t *testing.T) {
	startCalled := false
	stopCalled := false
	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				StartAppFunc: func(ctx context.Context, name string) error {
					startCalled = true
					return nil
				},
				StopAppFunc: func(ctx context.Context, name string) error {
					stopCalled = true
					return nil
				},
				GetAppFunc: func(ctx context.Context, name string) (*truenas.App, error) {
					return &truenas.App{Name: "myapp", State: "RUNNING"}, nil
				},
			},
		}},
	}

	schemaResp := getAppResourceSchema(t)

	// Both state and plan have same restart_triggers - no restart needed
	triggers := map[string]interface{}{"config_checksum": "same_checksum"}
	stateValue := createAppResourceModelValueWithTriggers(
		"myapp", "myapp", true, nil, "RUNNING", float64(120), "RUNNING",
		triggers,
	)
	planValue := createAppResourceModelValueWithTriggers(
		"myapp", "myapp", true, nil, "RUNNING", float64(120), nil,
		triggers,
	)

	req := resource.UpdateRequest{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    stateValue,
		},
		Plan: tfsdk.Plan{
			Schema: schemaResp.Schema,
			Raw:    planValue,
		},
	}

	resp := &resource.UpdateResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}

	r.Update(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics)
	}

	// No restart should be triggered when triggers don't change
	if startCalled || stopCalled {
		t.Error("expected no API calls when restart_triggers unchanged")
	}
}

func TestAppResource_Update_RestartTriggersStoppedAppNoRestart(t *testing.T) {
	startCalled := false
	stopCalled := false
	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				StartAppFunc: func(ctx context.Context, name string) error {
					startCalled = true
					return nil
				},
				StopAppFunc: func(ctx context.Context, name string) error {
					stopCalled = true
					return nil
				},
				GetAppFunc: func(ctx context.Context, name string) (*truenas.App, error) {
					return &truenas.App{Name: "myapp", State: "STOPPED"}, nil
				},
			},
		}},
	}

	schemaResp := getAppResourceSchema(t)

	// Triggers changed, but app is STOPPED - no restart needed
	stateValue := createAppResourceModelValueWithTriggers(
		"myapp", "myapp", true, nil, "STOPPED", float64(120), "STOPPED",
		map[string]interface{}{"config_checksum": "old_checksum"},
	)
	planValue := createAppResourceModelValueWithTriggers(
		"myapp", "myapp", true, nil, "STOPPED", float64(120), nil,
		map[string]interface{}{"config_checksum": "new_checksum"},
	)

	req := resource.UpdateRequest{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    stateValue,
		},
		Plan: tfsdk.Plan{
			Schema: schemaResp.Schema,
			Raw:    planValue,
		},
	}

	resp := &resource.UpdateResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}

	r.Update(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics)
	}

	// No restart should be triggered when app is stopped
	if startCalled || stopCalled {
		t.Error("expected no API calls for stopped app")
	}
}

func TestAppResource_Read_PreservesRestartTriggers(t *testing.T) {
	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				GetAppWithConfigFunc: func(ctx context.Context, name string) (*truenas.App, error) {
					return &truenas.App{
						Name:      "myapp",
						State:     "RUNNING",
						CustomApp: true,
						Config:    map[string]any{},
					}, nil
				},
			},
		}},
	}

	schemaResp := getAppResourceSchema(t)

	// Prior state has restart_triggers set
	stateValue := createAppResourceModelValueWithTriggers(
		"myapp", "myapp", true, nil, "RUNNING", float64(120), "RUNNING",
		map[string]interface{}{"config_checksum": "abc123"},
	)

	req := resource.ReadRequest{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    stateValue,
		},
	}

	resp := &resource.ReadResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}

	r.Read(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics)
	}

	var model AppResourceModel
	diags := resp.State.Get(context.Background(), &model)
	if diags.HasError() {
		t.Fatalf("failed to get state: %v", diags)
	}

	// restart_triggers should be preserved from prior state
	if model.RestartTriggers.IsNull() {
		t.Error("expected restart_triggers to be preserved, got null")
	}

	// Verify the trigger value is preserved
	triggers := make(map[string]string)
	diags = model.RestartTriggers.ElementsAs(context.Background(), &triggers, false)
	if diags.HasError() {
		t.Fatalf("failed to get restart_triggers: %v", diags)
	}
	if triggers["config_checksum"] != "abc123" {
		t.Errorf("expected config_checksum 'abc123', got %q", triggers["config_checksum"])
	}
}

func TestAppResource_Update_RestartTriggersAddedFirstTime(t *testing.T) {
	startCalled := false
	stopCalled := false
	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				StartAppFunc: func(ctx context.Context, name string) error {
					startCalled = true
					return nil
				},
				StopAppFunc: func(ctx context.Context, name string) error {
					stopCalled = true
					return nil
				},
				GetAppFunc: func(ctx context.Context, name string) (*truenas.App, error) {
					return &truenas.App{Name: "myapp", State: "RUNNING"}, nil
				},
			},
		}},
	}

	schemaResp := getAppResourceSchema(t)

	// Current state: no restart_triggers (null)
	stateValue := createAppResourceModelValueWithTriggers(
		"myapp", "myapp", true, nil, "RUNNING", float64(120), "RUNNING",
		nil, // null triggers
	)

	// Plan: has restart_triggers (first time adding them)
	planValue := createAppResourceModelValueWithTriggers(
		"myapp", "myapp", true, nil, "RUNNING", float64(120), nil,
		map[string]interface{}{"config_checksum": "abc123"},
	)

	req := resource.UpdateRequest{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    stateValue,
		},
		Plan: tfsdk.Plan{
			Schema: schemaResp.Schema,
			Raw:    planValue,
		},
	}

	resp := &resource.UpdateResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}

	r.Update(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics)
	}

	// No app.stop or app.start should be called when triggers go from null to value
	if startCalled || stopCalled {
		t.Error("expected no restart when adding triggers first time")
	}
}

func TestAppResource_Update_RestartTriggersRemoved(t *testing.T) {
	startCalled := false
	stopCalled := false
	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				StartAppFunc: func(ctx context.Context, name string) error {
					startCalled = true
					return nil
				},
				StopAppFunc: func(ctx context.Context, name string) error {
					stopCalled = true
					return nil
				},
				GetAppFunc: func(ctx context.Context, name string) (*truenas.App, error) {
					return &truenas.App{Name: "myapp", State: "RUNNING"}, nil
				},
			},
		}},
	}

	schemaResp := getAppResourceSchema(t)

	// Current state: has restart_triggers
	stateValue := createAppResourceModelValueWithTriggers(
		"myapp", "myapp", true, nil, "RUNNING", float64(120), "RUNNING",
		map[string]interface{}{"config_checksum": "abc123"},
	)

	// Plan: no restart_triggers (removed)
	planValue := createAppResourceModelValueWithTriggers(
		"myapp", "myapp", true, nil, "RUNNING", float64(120), nil,
		nil, // null triggers
	)

	req := resource.UpdateRequest{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    stateValue,
		},
		Plan: tfsdk.Plan{
			Schema: schemaResp.Schema,
			Raw:    planValue,
		},
	}

	resp := &resource.UpdateResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}

	r.Update(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics)
	}

	// No app.stop or app.start should be called when triggers are removed
	if startCalled || stopCalled {
		t.Error("expected no restart when removing triggers")
	}
}

func TestAppResource_Update_RestartTriggersStopError(t *testing.T) {
	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				StopAppFunc: func(ctx context.Context, name string) error {
					return errors.New("stop failed: container busy")
				},
				GetAppFunc: func(ctx context.Context, name string) (*truenas.App, error) {
					return &truenas.App{Name: "myapp", State: "RUNNING"}, nil
				},
			},
		}},
	}

	schemaResp := getAppResourceSchema(t)

	// Current state: has restart_triggers with old checksum
	stateValue := createAppResourceModelValueWithTriggers(
		"myapp", "myapp", true, nil, "RUNNING", float64(120), "RUNNING",
		map[string]interface{}{"config_checksum": "old_checksum"},
	)

	// Plan: has restart_triggers with new checksum (trigger change)
	planValue := createAppResourceModelValueWithTriggers(
		"myapp", "myapp", true, nil, "RUNNING", float64(120), nil,
		map[string]interface{}{"config_checksum": "new_checksum"},
	)

	req := resource.UpdateRequest{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    stateValue,
		},
		Plan: tfsdk.Plan{
			Schema: schemaResp.Schema,
			Raw:    planValue,
		},
	}

	resp := &resource.UpdateResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}

	r.Update(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when app.stop fails")
	}

	// Verify the error message contains expected text
	foundError := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Unable to Stop App for Restart") {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Error("expected error with 'Unable to Stop App for Restart' message")
	}
}

func TestAppResource_Update_RestartTriggersStartError(t *testing.T) {
	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				StopAppFunc: func(ctx context.Context, name string) error {
					return nil // stop succeeds
				},
				StartAppFunc: func(ctx context.Context, name string) error {
					return errors.New("start failed: port already in use")
				},
				GetAppFunc: func(ctx context.Context, name string) (*truenas.App, error) {
					return &truenas.App{Name: "myapp", State: "RUNNING"}, nil
				},
			},
		}},
	}

	schemaResp := getAppResourceSchema(t)

	// Current state: has restart_triggers with old checksum
	stateValue := createAppResourceModelValueWithTriggers(
		"myapp", "myapp", true, nil, "RUNNING", float64(120), "RUNNING",
		map[string]interface{}{"config_checksum": "old_checksum"},
	)

	// Plan: has restart_triggers with new checksum (trigger change)
	planValue := createAppResourceModelValueWithTriggers(
		"myapp", "myapp", true, nil, "RUNNING", float64(120), nil,
		map[string]interface{}{"config_checksum": "new_checksum"},
	)

	req := resource.UpdateRequest{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    stateValue,
		},
		Plan: tfsdk.Plan{
			Schema: schemaResp.Schema,
			Raw:    planValue,
		},
	}

	resp := &resource.UpdateResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}

	r.Update(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when app.start fails")
	}

	// Verify the error message contains expected text
	foundError := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Unable to Start App for Restart") {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Error("expected error with 'Unable to Start App for Restart' message")
	}
}

// TestAppResource_Update_DesiredStateCasePreservation verifies that Update preserves
// the user's original case for desired_state (bug fix for "planned value does not
// match config value" errors).
func TestAppResource_Update_DesiredStateCasePreservation(t *testing.T) {
	r := &AppResource{
		BaseResource: BaseResource{services: &services.TrueNASServices{
			App: &truenas.MockAppService{
				StopAppFunc: func(ctx context.Context, name string) error {
					return nil
				},
				GetAppFunc: func(ctx context.Context, name string) (*truenas.App, error) {
					return &truenas.App{
						Name:      "myapp",
						State:     "STOPPED",
						CustomApp: true,
						Config:    map[string]any{},
					}, nil
				},
			},
		}},
	}

	schemaResp := getAppResourceSchema(t)

	// Prior state has uppercase "RUNNING"
	stateValue := newAppModelValue(appModelParams{
		ID:           "myapp",
		Name:         "myapp",
		CustomApp:    true,
		DesiredState: "RUNNING",
		StateTimeout: float64(120),
		State:        "RUNNING",
	})

	// Plan has lowercase "stopped" (user changed config to use lowercase)
	planValue := newAppModelValue(appModelParams{
		ID:           "myapp",
		Name:         "myapp",
		CustomApp:    true,
		DesiredState: "stopped",
		StateTimeout: float64(120),
	})

	req := resource.UpdateRequest{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    stateValue,
		},
		Plan: tfsdk.Plan{
			Schema: schemaResp.Schema,
			Raw:    planValue,
		},
	}

	resp := &resource.UpdateResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}

	r.Update(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics)
	}

	var model AppResourceModel
	diags := resp.State.Get(context.Background(), &model)
	if diags.HasError() {
		t.Fatalf("failed to get state: %v", diags)
	}

	// Critical: desired_state should preserve user's lowercase "stopped"
	if model.DesiredState.ValueString() != "stopped" {
		t.Errorf("expected lowercase 'stopped' to be preserved, got %q", model.DesiredState.ValueString())
	}

	// State should reflect the actual API state
	if model.State.ValueString() != "STOPPED" {
		t.Errorf("expected state 'STOPPED', got %q", model.State.ValueString())
	}
}

func TestAppResource_buildCreateParams_CatalogApp(t *testing.T) {
	values := types.ObjectValueMust(
		map[string]attr.Type{"replicas": types.Int64Type},
		map[string]attr.Value{"replicas": types.Int64Value(2)},
	)

	r := &AppResource{}
	model := &AppResourceModel{
		Name:       types.StringValue("plex"),
		CustomApp:  types.BoolValue(false),
		CatalogApp: types.StringValue("plex"),
		Train:      types.StringValue("stable"),
		Version:    types.StringValue("1.2.3"),
		Values:     types.DynamicValue(values),
	}

	params := r.buildCreateParams(context.Background(), model)

	if params["app_name"] != "plex" {
		t.Fatalf("expected app_name plex, got %#v", params["app_name"])
	}
	if params["catalog_app"] != "plex" {
		t.Fatalf("expected catalog_app plex, got %#v", params["catalog_app"])
	}
	if params["train"] != "stable" {
		t.Fatalf("expected train stable, got %#v", params["train"])
	}
	if params["version"] != "1.2.3" {
		t.Fatalf("expected version 1.2.3, got %#v", params["version"])
	}

	valuesMap, ok := params["values"].(map[string]any)
	if !ok {
		t.Fatalf("expected values map, got %T", params["values"])
	}
	if valuesMap["replicas"] != int64(2) {
		t.Fatalf("expected replicas 2, got %#v", valuesMap["replicas"])
	}
}

func TestAppResource_Read_RawClientCatalogAppMapsValues(t *testing.T) {
	r := &AppResource{
		BaseResource: BaseResource{
			client: &client.MockClient{
				CallFunc: func(ctx context.Context, method string, params any) (json.RawMessage, error) {
					if method != "app.query" {
						t.Fatalf("unexpected method %q", method)
					}

					return json.RawMessage(`[
						{
							"id": "plex",
							"name": "plex",
							"state": "RUNNING",
							"custom_app": false,
							"version": "2.0.0",
							"human_version": "2.0.0_1.0.0",
							"upgrade_available": true,
							"latest_version": "2.1.0",
							"latest_app_version": "2.1.0",
							"image_updates_available": false,
							"migrated": false,
							"metadata": {"icon": "plex.svg"},
							"active_workloads": {"containers": 1},
							"notes": "hello",
							"portals": {"web": "http://plex.local"},
							"version_details": {"healthy": true},
							"config": {"resources": {"limits": {"cpu": 2}}}
						}
					]`), nil
				},
			},
		},
	}

	schemaResp := getAppResourceSchema(t)
	stateValue := newAppModelValue(appModelParams{
		ID:         "plex",
		Name:       "plex",
		CustomApp:  false,
		CatalogApp: "plex",
		Train:      "stable",
		Version:    "latest",
		State:      "RUNNING",
	})

	req := resource.ReadRequest{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    stateValue,
		},
	}
	resp := &resource.ReadResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}

	r.Read(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics)
	}

	var model AppResourceModel
	diags := resp.State.Get(context.Background(), &model)
	if diags.HasError() {
		t.Fatalf("failed to get state: %v", diags)
	}

	if model.CustomApp.ValueBool() {
		t.Fatal("expected catalog app")
	}
	if model.Values.IsNull() {
		t.Fatal("expected values to be populated from API config")
	}
	if model.CustomComposeConfigString.IsNull() == false {
		t.Fatal("expected custom compose config string to stay null for catalog app")
	}
	if model.InstalledVersion.ValueString() != "2.0.0" {
		t.Fatalf("expected installed version 2.0.0, got %q", model.InstalledVersion.ValueString())
	}
	if !model.UpgradeAvailable.ValueBool() {
		t.Fatal("expected upgrade_available to be true")
	}
}

func TestAppResource_Read_RawClientCatalogAppPreservesPriorValues(t *testing.T) {
	r := &AppResource{
		BaseResource: BaseResource{
			client: &client.MockClient{
				CallFunc: func(ctx context.Context, method string, params any) (json.RawMessage, error) {
					return json.RawMessage(`[
						{
							"id": "minio",
							"name": "minio",
							"state": "RUNNING",
							"custom_app": false,
							"config": {
								"minio": {"existingSecret": "redacted"},
								"network": {"api_port": {"port_number": 9000}}
							}
						}
					]`), nil
				},
			},
		},
	}

	schemaResp := getAppResourceSchema(t)
	plannedValues, diags := types.ObjectValue(
		map[string]attr.Type{
			"minio": types.ObjectType{AttrTypes: map[string]attr.Type{
				"rootUser": types.StringType,
			}},
			"network": types.ObjectType{AttrTypes: map[string]attr.Type{
				"api_port": types.ObjectType{AttrTypes: map[string]attr.Type{
					"port_number": types.Int64Type,
				}},
			}},
		},
		map[string]attr.Value{
			"minio": types.ObjectValueMust(
				map[string]attr.Type{"rootUser": types.StringType},
				map[string]attr.Value{"rootUser": types.StringValue("admin")},
			),
			"network": types.ObjectValueMust(
				map[string]attr.Type{
					"api_port": types.ObjectType{AttrTypes: map[string]attr.Type{
						"port_number": types.Int64Type,
					}},
				},
				map[string]attr.Value{
					"api_port": types.ObjectValueMust(
						map[string]attr.Type{"port_number": types.Int64Type},
						map[string]attr.Value{"port_number": types.Int64Value(9000)},
					),
				},
			),
		},
	)
	if diags.HasError() {
		t.Fatalf("failed to build planned values: %v", diags)
	}

	priorState := tfsdk.State{Schema: schemaResp.Schema}
	setDiags := priorState.Set(context.Background(), &AppResourceModel{
		ID:              types.StringValue("minio"),
		Name:            types.StringValue("minio"),
		CustomApp:       types.BoolValue(false),
		CatalogApp:      types.StringValue("minio"),
		Train:           types.StringValue("stable"),
		Version:         types.StringValue("latest"),
		Values:          types.DynamicValue(plannedValues),
		State:           types.StringValue("RUNNING"),
		RestartTriggers: types.MapNull(types.StringType),
	})
	if setDiags.HasError() {
		t.Fatalf("failed to build prior state: %v", setDiags)
	}

	req := resource.ReadRequest{
		State: priorState,
	}
	resp := &resource.ReadResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}

	r.Read(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics)
	}

	var model AppResourceModel
	diags = resp.State.Get(context.Background(), &model)
	if diags.HasError() {
		t.Fatalf("failed to get state: %v", diags)
	}

	valuesObj, ok := model.Values.UnderlyingValue().(types.Object)
	if !ok {
		t.Fatalf("expected object underlying value, got %T", model.Values.UnderlyingValue())
	}
	minioVal, ok := valuesObj.Attributes()["minio"].(types.Object)
	if !ok {
		t.Fatalf("expected minio object, got %T", valuesObj.Attributes()["minio"])
	}
	rootUser, ok := minioVal.Attributes()["rootUser"].(types.String)
	if !ok {
		t.Fatalf("expected rootUser string, got %T", minioVal.Attributes()["rootUser"])
	}
	if rootUser.ValueString() != "admin" {
		t.Fatalf("expected preserved rootUser admin, got %q", rootUser.ValueString())
	}

	if model.Config.IsNull() {
		t.Fatal("expected API config to still be populated in computed config field")
	}
}
