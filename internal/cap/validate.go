package cap

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

// ValidateAgainstSchema 校验 payload 是否符合能力声明的 JSON Schema。
// schema 为空时跳过校验。返回清晰错误,便于调用方修正入参。
func ValidateAgainstSchema(schema string, payload []byte) error {
	if strings.TrimSpace(schema) == "" {
		return nil
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("capability.json", strings.NewReader(schema)); err != nil {
		// 能力方 schema 本身非法:跳过校验(不阻塞调用),由调用方自行负责。
		return nil
	}
	sch, err := compiler.Compile("capability.json")
	if err != nil {
		return nil
	}
	var v interface{}
	if err := json.Unmarshal(payload, &v); err != nil {
		return fmt.Errorf("payload is not valid JSON: %v", err)
	}
	if err := sch.Validate(v); err != nil {
		return fmt.Errorf("payload invalid per capability schema: %v", err)
	}
	return nil
}
