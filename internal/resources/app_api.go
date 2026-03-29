package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"

	truenas "github.com/deevus/truenas-go"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type appEntryResponse struct {
	ID                    string  `json:"id"`
	Name                  string  `json:"name"`
	State                 string  `json:"state"`
	UpgradeAvailable      bool    `json:"upgrade_available"`
	LatestVersion         *string `json:"latest_version"`
	LatestAppVersion      *string `json:"latest_app_version"`
	ImageUpdatesAvailable bool    `json:"image_updates_available"`
	CustomApp             bool    `json:"custom_app"`
	Migrated              bool    `json:"migrated"`
	HumanVersion          string  `json:"human_version"`
	Version               string  `json:"version"`
	Metadata              any     `json:"metadata"`
	ActiveWorkloads       any     `json:"active_workloads"`
	Notes                 *string `json:"notes"`
	Portals               any     `json:"portals"`
	VersionDetails        any     `json:"version_details"`
	Config                any     `json:"config"`
}

func (r *AppResource) createAppEntry(ctx context.Context, data *AppResourceModel) (*appEntryResponse, error) {
	if r.client == nil {
		opts := r.buildCreateOpts(data)
		app, err := r.services.App.CreateApp(ctx, opts)
		if err != nil {
			return nil, err
		}
		return &appEntryResponse{
			ID:        app.Name,
			Name:      app.Name,
			State:     app.State,
			CustomApp: app.CustomApp,
			Version:   app.Version,
		}, nil
	}

	_, err := r.client.CallAndWait(ctx, "app.create", r.buildCreateParams(ctx, data))
	if err != nil {
		return nil, err
	}

	return r.readAppEntry(ctx, data.Name.ValueString())
}

func (r *AppResource) updateAppEntry(ctx context.Context, data *AppResourceModel) (*appEntryResponse, error) {
	if r.client == nil {
		app, err := r.services.App.UpdateApp(ctx, data.Name.ValueString(), r.buildUpdateOpts(data))
		if err != nil {
			return nil, err
		}
		return &appEntryResponse{
			ID:        app.Name,
			Name:      app.Name,
			State:     app.State,
			CustomApp: app.CustomApp,
			Version:   app.Version,
		}, nil
	}

	params := []any{data.Name.ValueString(), r.buildUpdateParams(ctx, data)}
	if _, err := r.client.CallAndWait(ctx, "app.update", params); err != nil {
		return nil, err
	}

	return r.readAppEntry(ctx, data.Name.ValueString())
}

func (r *AppResource) readAppEntry(ctx context.Context, name string) (*appEntryResponse, error) {
	if r.client == nil {
		app, err := r.services.App.GetAppWithConfig(ctx, name)
		if err != nil {
			return nil, err
		}
		if app == nil {
			return nil, nil
		}
		return &appEntryResponse{
			ID:               app.Name,
			Name:             app.Name,
			State:            app.State,
			CustomApp:        app.CustomApp,
			Version:          app.Version,
			HumanVersion:     app.HumanVersion,
			UpgradeAvailable: app.UpgradeAvailable,
			LatestVersion:    stringPtrOrNil(app.LatestVersion),
			ActiveWorkloads:  appActiveWorkloadsToAny(app.ActiveWorkloads),
			Config:           app.Config,
		}, nil
	}

	params := []any{
		[][]any{{"name", "=", name}},
		map[string]any{"extra": map[string]any{"retrieve_config": true}},
	}
	result, err := r.client.Call(ctx, "app.query", params)
	if err != nil {
		return nil, err
	}

	var entries []appEntryResponse
	if err := json.Unmarshal(result, &entries); err != nil {
		return nil, fmt.Errorf("parse app.query response: %w", err)
	}

	if len(entries) == 0 {
		return nil, nil
	}

	return &entries[0], nil
}

func (r *AppResource) buildCreateParams(ctx context.Context, data *AppResourceModel) map[string]any {
	params := map[string]any{
		"app_name":   data.Name.ValueString(),
		"custom_app": data.CustomApp.ValueBool(),
	}

	if value := dynamicValueToAny(ctx, data.Values); value != nil {
		params["values"] = value
	}
	if value := dynamicValueToAny(ctx, data.CustomComposeConfig); value != nil {
		params["custom_compose_config"] = value
	}
	if compose := r.effectiveComposeConfigString(data); compose != "" {
		params["custom_compose_config_string"] = compose
	}
	if !data.CatalogApp.IsNull() && !data.CatalogApp.IsUnknown() {
		params["catalog_app"] = data.CatalogApp.ValueString()
	}
	if !data.Train.IsNull() && !data.Train.IsUnknown() {
		params["train"] = data.Train.ValueString()
	}
	if !data.Version.IsNull() && !data.Version.IsUnknown() {
		params["version"] = data.Version.ValueString()
	}

	return params
}

func (r *AppResource) buildUpdateParams(ctx context.Context, data *AppResourceModel) map[string]any {
	params := map[string]any{}

	if value := dynamicValueToAny(ctx, data.Values); value != nil {
		params["values"] = value
	}
	if value := dynamicValueToAny(ctx, data.CustomComposeConfig); value != nil {
		params["custom_compose_config"] = value
	}
	if compose := r.effectiveComposeConfigString(data); compose != "" {
		params["custom_compose_config_string"] = compose
	}

	return params
}

func (r *AppResource) buildCreateOpts(data *AppResourceModel) truenas.CreateAppOpts {
	opts := truenas.CreateAppOpts{
		Name:      data.Name.ValueString(),
		CustomApp: data.CustomApp.ValueBool(),
	}

	if compose := r.effectiveComposeConfigString(data); compose != "" {
		opts.CustomComposeConfig = compose
	}

	return opts
}

func (r *AppResource) buildUpdateOpts(data *AppResourceModel) truenas.UpdateAppOpts {
	opts := truenas.UpdateAppOpts{}

	if compose := r.effectiveComposeConfigString(data); compose != "" {
		opts.CustomComposeConfig = compose
	}

	return opts
}

func (r *AppResource) effectiveComposeConfigString(data *AppResourceModel) string {
	if !data.CustomComposeConfigString.IsNull() && !data.CustomComposeConfigString.IsUnknown() {
		return data.CustomComposeConfigString.ValueString()
	}
	if !data.ComposeConfig.IsNull() && !data.ComposeConfig.IsUnknown() {
		return data.ComposeConfig.ValueString()
	}
	return ""
}

func dynamicValueToAny(ctx context.Context, value types.Dynamic) any {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}

	return attrValueToAny(ctx, value.UnderlyingValue())
}

func attrValueToAny(ctx context.Context, value attr.Value) any {
	if value == nil || value.IsNull() || value.IsUnknown() {
		return nil
	}

	switch v := value.(type) {
	case types.Dynamic:
		return dynamicValueToAny(ctx, v)
	case types.String:
		return v.ValueString()
	case types.Bool:
		return v.ValueBool()
	case types.Int64:
		return v.ValueInt64()
	case types.Number:
		f := v.ValueBigFloat()
		if i, acc := f.Int64(); acc == big.Exact {
			return i
		}
		floatVal, _ := f.Float64()
		return floatVal
	case types.List:
		values := make([]any, 0, len(v.Elements()))
		for _, elem := range v.Elements() {
			values = append(values, attrValueToAny(ctx, elem))
		}
		return values
	case types.Set:
		values := make([]any, 0, len(v.Elements()))
		for _, elem := range v.Elements() {
			values = append(values, attrValueToAny(ctx, elem))
		}
		return values
	case types.Map:
		values := make(map[string]any, len(v.Elements()))
		for key, elem := range v.Elements() {
			values[key] = attrValueToAny(ctx, elem)
		}
		return values
	case types.Object:
		values := make(map[string]any, len(v.Attributes()))
		for key, elem := range v.Attributes() {
			values[key] = attrValueToAny(ctx, elem)
		}
		return values
	default:
		return nil
	}
}

func anyToDynamicValue(value any) types.Dynamic {
	attrValue := anyToAttrValue(value)
	if attrValue == nil {
		return types.DynamicNull()
	}
	return types.DynamicValue(attrValue)
}

func anyToAttrValue(value any) attr.Value {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		return types.StringValue(v)
	case bool:
		return types.BoolValue(v)
	case int:
		return types.Int64Value(int64(v))
	case int8:
		return types.Int64Value(int64(v))
	case int16:
		return types.Int64Value(int64(v))
	case int32:
		return types.Int64Value(int64(v))
	case int64:
		return types.Int64Value(v)
	case float32:
		return types.NumberValue(big.NewFloat(float64(v)))
	case float64:
		return types.NumberValue(big.NewFloat(v))
	case []any:
		elements := make([]attr.Value, 0, len(v))
		var elementType attr.Type = types.StringType
		if len(v) > 0 {
			first := anyToAttrValue(v[0])
			if first != nil {
				elementType = first.Type(context.Background())
			}
		}
		for _, item := range v {
			attrValue := anyToAttrValue(item)
			if attrValue == nil {
				elements = append(elements, types.StringNull())
				continue
			}
			elements = append(elements, attrValue)
		}
		listValue, diags := types.ListValue(elementType, elements)
		if diags.HasError() {
			return nil
		}
		return listValue
	case []string:
		elements := make([]attr.Value, 0, len(v))
		for _, item := range v {
			elements = append(elements, types.StringValue(item))
		}
		listValue, diags := types.ListValue(types.StringType, elements)
		if diags.HasError() {
			return nil
		}
		return listValue
	case map[string]any:
		attrTypes := make(map[string]attr.Type, len(v))
		attrs := make(map[string]attr.Value, len(v))
		for key, item := range v {
			attrValue := anyToAttrValue(item)
			if attrValue == nil {
				attrValue = types.DynamicNull()
			}
			attrTypes[key] = attrValue.Type(context.Background())
			attrs[key] = attrValue
		}
		objectValue, diags := types.ObjectValue(attrTypes, attrs)
		if diags.HasError() {
			return nil
		}
		return objectValue
	default:
		jsonBytes, err := json.Marshal(v)
		if err != nil {
			return nil
		}

		var generic any
		if err := json.Unmarshal(jsonBytes, &generic); err != nil {
			return nil
		}

		return anyToAttrValue(generic)
	}
}

func stringPtrOrNil(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func appActiveWorkloadsToAny(workloads truenas.AppActiveWorkloads) map[string]any {
	usedPorts := make([]any, 0, len(workloads.UsedPorts))
	for _, port := range workloads.UsedPorts {
		hostPorts := []any{
			map[string]any{
				"host_port": port.HostPort,
				"host_ip":   "",
			},
		}
		usedPorts = append(usedPorts, map[string]any{
			"container_port": port.ContainerPort,
			"protocol":       port.Protocol,
			"host_ports":     hostPorts,
		})
	}

	containerDetails := make([]any, 0, len(workloads.ContainerDetails))
	for _, container := range workloads.ContainerDetails {
		containerDetails = append(containerDetails, map[string]any{
			"id":           container.ID,
			"service_name": container.ServiceName,
			"image":        container.Image,
			"state":        string(container.State),
		})
	}

	return map[string]any{
		"containers":        workloads.Containers,
		"used_ports":        usedPorts,
		"used_host_ips":     []any{},
		"container_details": containerDetails,
	}
}
