package cap

import "testing"

func TestValidateAgainstSchema(t *testing.T) {
	schema := `{"type":"object","properties":{"who":{"type":"string"}},"required":["who"]}`

	if err := ValidateAgainstSchema(schema, []byte(`{"who":"world"}`)); err != nil {
		t.Errorf("valid payload should pass, got %v", err)
	}
	if err := ValidateAgainstSchema(schema, []byte(`{"lang":"zh"}`)); err == nil {
		t.Error("missing required field should fail")
	}
	if err := ValidateAgainstSchema(schema, []byte(`{"who":123}`)); err == nil {
		t.Error("wrong type should fail")
	}
	// 空 schema → 跳过校验
	if err := ValidateAgainstSchema("", []byte(`garbage`)); err != nil {
		t.Errorf("empty schema should skip validation, got %v", err)
	}
}
