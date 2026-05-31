package plugin

import "testing"

func TestConfigSchemaFieldByKey(t *testing.T) {
	schema := ConfigSchema{
		Fields: []ConfigField{
			{Key: "enabled", Type: ConfigTypeBool, Label: "Enabled"},
			{Key: "limit", Type: ConfigTypeInt, Label: "Limit", Default: 100},
		},
	}

	f, ok := schema.FieldByKey("limit")
	if !ok {
		t.Fatal("expected field limit")
	}
	if f.Type != ConfigTypeInt {
		t.Fatalf("type = %q, want int", f.Type)
	}
	if f.Default != 100 {
		t.Fatalf("default = %v, want 100", f.Default)
	}

	_, ok = schema.FieldByKey("missing")
	if ok {
		t.Fatal("expected missing field to be absent")
	}
}

func TestConfigFieldTypes(t *testing.T) {
	types := []ConfigFieldType{
		ConfigTypeString,
		ConfigTypeInt,
		ConfigTypeBool,
		ConfigTypeIntList,
		ConfigTypeStringList,
	}
	for _, typ := range types {
		if typ == "" {
			t.Fatal("empty config field type")
		}
	}
}
