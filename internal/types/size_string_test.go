package types

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

func TestSizeStringValue_StringSemanticEquals(t *testing.T) {
	tests := []struct {
		name     string
		value1   string
		value2   string
		expected bool
	}{
		// Same values
		{"same bytes", "1000000000", "1000000000", true},
		{"same human readable", "1GB", "1GB", true},

		// Semantically equal (same byte value)
		{"bytes vs GB", "1000000000", "1GB", true},
		{"GB vs bytes", "1GB", "1000000000", true},
		{"2TB vs bytes", "2000000000000", "2TB", true},

		// Not equal
		{"different values", "1GB", "2GB", false},
		{"1GB vs 1GiB", "1GB", "1GiB", false}, // 1000000000 vs 1073741824

		// Zero
		{"zero", "0", "0", true},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v1 := NewSizeStringValue(tt.value1)
			v2 := NewSizeStringValue(tt.value2)

			equal, diags := v1.StringSemanticEquals(ctx, v2)
			if diags.HasError() {
				t.Fatalf("unexpected error: %v", diags)
			}
			if equal != tt.expected {
				t.Errorf("StringSemanticEquals(%q, %q) = %v, want %v", tt.value1, tt.value2, equal, tt.expected)
			}
		})
	}
}

func TestSizeStringValue_StringSemanticEquals_NullUnknown(t *testing.T) {
	ctx := context.Background()

	// Both null
	v1 := NewSizeStringNull()
	v2 := NewSizeStringNull()
	equal, diags := v1.StringSemanticEquals(ctx, v2)
	if diags.HasError() {
		t.Fatalf("unexpected error: %v", diags)
	}
	if !equal {
		t.Error("expected null == null")
	}

	// One null
	v1 = NewSizeStringValue("1GB")
	v2 = NewSizeStringNull()
	equal, diags = v1.StringSemanticEquals(ctx, v2)
	if diags.HasError() {
		t.Fatalf("unexpected error: %v", diags)
	}
	if equal {
		t.Error("expected value != null")
	}

	// One unknown
	v1 = NewSizeStringValue("1GB")
	v2 = NewSizeStringUnknown()
	equal, diags = v1.StringSemanticEquals(ctx, v2)
	if diags.HasError() {
		t.Fatalf("unexpected error: %v", diags)
	}
	if equal {
		t.Error("expected value != unknown")
	}
}

func TestSizeStringType_ValueFromString(t *testing.T) {
	ctx := context.Background()
	st := SizeStringType{}

	sv := basetypes.NewStringValue("1GB")
	result, diags := st.ValueFromString(ctx, sv)
	if diags.HasError() {
		t.Fatalf("unexpected error: %v", diags)
	}

	ssv, ok := result.(SizeStringValue)
	if !ok {
		t.Fatalf("expected SizeStringValue, got %T", result)
	}
	if ssv.ValueString() != "1GB" {
		t.Errorf("expected '1GB', got %q", ssv.ValueString())
	}
}

func TestSizeStringType_Equal(t *testing.T) {
	t1 := SizeStringType{}
	t2 := SizeStringType{}

	if !t1.Equal(t2) {
		t.Error("expected SizeStringType to equal itself")
	}

	if t1.Equal(basetypes.StringType{}) {
		t.Error("expected SizeStringType to not equal StringType")
	}
}

func TestSizeStringType_String(t *testing.T) {
	st := SizeStringType{}
	if st.String() != "SizeStringType" {
		t.Errorf("expected 'SizeStringType', got %q", st.String())
	}
}

func TestSizeStringType_ImplementsInterface(t *testing.T) {
	t.Parallel()

	var _ attr.Type = SizeStringType{}
	var _ basetypes.StringTypable = SizeStringType{}
}

func TestSizeStringValue_ImplementsInterface(t *testing.T) {
	t.Parallel()

	var _ attr.Value = SizeStringValue{}
	var _ basetypes.StringValuable = SizeStringValue{}
	var _ basetypes.StringValuableWithSemanticEquals = SizeStringValue{}
}
