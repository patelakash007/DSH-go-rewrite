package tests

import (
	"bytes"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/tools"
)

func TestSchemaCompileAndValidate(t *testing.T) {
	schema, err := tools.Compile([]byte(`{
		"type": "object",
		"properties": {
			"name": {"type": "string", "minLength": 1},
			"count": {"type": "integer", "minimum": 0},
			"tags": {"type": "array", "items": {"type": "string"}, "minItems": 1}
		},
		"required": ["name"]
	}`))
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	ok := map[string]any{"name": "hello", "count": 3, "tags": []any{"a", "b"}}
	if err := schema.Validate(ok); err != nil {
		t.Fatalf("valid value should pass: %v", err)
	}

	if err := schema.Validate(map[string]any{"count": 1}); err == nil {
		t.Fatal("missing required field should fail validation")
	}

	if err := schema.Validate(map[string]any{"name": 42}); err == nil {
		t.Fatal("type error should fail validation")
	}

	if err := schema.Validate(map[string]any{"name": ""}); err == nil {
		t.Fatal("minLength violation should fail validation")
	}

	if err := schema.Validate(map[string]any{"name": "x", "tags": []any{}}); err == nil {
		t.Fatal("minItems violation should fail validation")
	}
}

func TestSchemaAdditionalPropertiesFalse(t *testing.T) {
	schema, err := tools.Compile([]byte(`{
		"type": "object",
		"properties": {"known": {"type": "string"}},
		"additionalProperties": false,
		"required": ["known"]
	}`))
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	if err := schema.Validate(map[string]any{"known": "v"}); err != nil {
		t.Fatalf("declared properties only should pass: %v", err)
	}
	if err := schema.Validate(map[string]any{"known": "v", "unknown": 1}); err == nil {
		t.Fatal("undeclared property should fail when additionalProperties=false")
	}
}

func TestSchemaEnumAndConst(t *testing.T) {
	enumSchema, err := tools.Compile([]byte(`{"type": "string", "enum": ["low", "medium", "high"]}`))
	if err != nil {
		t.Fatalf("Compile enum failed: %v", err)
	}
	if err := enumSchema.Validate("medium"); err != nil {
		t.Fatalf("enum match should pass: %v", err)
	}
	if err := enumSchema.Validate("ultra"); err == nil {
		t.Fatal("enum mismatch should fail")
	}

	constSchema, err := tools.Compile([]byte(`{"type": "string", "const": "fixed"}`))
	if err != nil {
		t.Fatalf("Compile const failed: %v", err)
	}
	if err := constSchema.Validate("fixed"); err != nil {
		t.Fatalf("const match should pass: %v", err)
	}
	if err := constSchema.Validate("other"); err == nil {
		t.Fatal("const mismatch should fail")
	}
}

func TestSchemaOneOf(t *testing.T) {
	schema, err := tools.Compile([]byte(`{
		"oneOf": [
			{"type": "string", "minLength": 5},
			{"type": "integer", "minimum": 100}
		]
	}`))
	if err != nil {
		t.Fatalf("Compile oneOf failed: %v", err)
	}

	if err := schema.Validate("hello"); err != nil {
		t.Fatalf("string branch should pass: %v", err)
	}
	if err := schema.Validate(200); err != nil {
		t.Fatalf("integer branch should pass: %v", err)
	}
	if err := schema.Validate(50); err == nil {
		t.Fatal("should fail when neither branch matches")
	}
	if err := schema.Validate(true); err == nil {
		t.Fatal("should fail when types do not match")
	}
}

func TestSchemaRejectUnsupportedKeys(t *testing.T) {

	if _, err := tools.Compile([]byte(`{"$ref": "#/definitions/x"}`)); err == nil {
		t.Fatal("$ref should be rejected")
	}

	if _, err := tools.Compile([]byte(`{"type":"object","properties":{"a":{"allOf":[{"type":"string"}]}}}`)); err == nil {
		t.Fatal("nested allOf should be rejected")
	}

	if _, err := tools.Compile([]byte(`{invalid`)); err == nil {
		t.Fatal("invalid JSON should error")
	}

	if _, err := tools.Compile([]byte(`{"type":"array"}`)); err == nil {
		t.Fatal("array missing items should error")
	}
}

func TestSchemaDefaultExamples(t *testing.T) {
	schema, err := tools.Compile([]byte(`{
		"type": "string",
		"default": "dft",
		"examples": ["e1", "e2"]
	}`))
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	if schema.Default != "dft" {
		t.Fatalf("default not retained: %v", schema.Default)
	}
	if len(schema.Examples) != 2 {
		t.Fatalf("examples not retained: %v", schema.Examples)
	}
}

func TestSchemaInfer(t *testing.T) {
	type inner struct {
		ID    int    `json:"id"`
		Label string `json:"label,omitempty"`
	}
	type payload struct {
		Name  string  `json:"name"`
		Score float64 `json:"score"`
		Items []inner `json:"items"`
	}

	schema, err := tools.Infer(payload{})
	if err != nil {
		t.Fatalf("Infer failed: %v", err)
	}
	if schema.Type != tools.TypeObject {
		t.Fatalf("top level should be object: %v", schema.Type)
	}

	if len(schema.Required) != 3 {
		t.Fatalf("top level required should contain name/score/items: %v", schema.Required)
	}

	innerSchema := schema.Properties["items"].Items
	if len(innerSchema.Required) != 1 || innerSchema.Required[0] != "id" {
		t.Fatalf("inner required should only contain id (label has omitempty): %v", innerSchema.Required)
	}
	if schema.Properties["items"].Type != tools.TypeArray {
		t.Fatalf("items should be array: %v", schema.Properties["items"].Type)
	}
	if schema.Properties["items"].Items.Properties["id"].Type != tools.TypeInteger {
		t.Fatal("inner id should be integer")
	}

	val := map[string]any{
		"name":  "x",
		"score": 1.5,
		"items": []any{map[string]any{"id": 1}},
	}
	if err := schema.Validate(val); err != nil {
		t.Fatalf("inferred schema validation should pass: %v", err)
	}
}

func TestSchemaMarshalDeterministic(t *testing.T) {
	schema, err := tools.Compile([]byte(`{
		"type": "object",
		"properties": {
			"zeta": {"type": "string"},
			"alpha": {"type": "string"}
		},
		"required": ["alpha", "zeta"]
	}`))
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	b1, err := schema.Marshal()
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	b2, err := schema.Marshal()
	if err != nil {
		t.Fatalf("second Marshal failed: %v", err)
	}
	if string(b1) != string(b2) {
		t.Fatal("Marshal output non-deterministic")
	}

	if bytes.Index(b1, []byte("alpha")) == -1 || bytes.Index(b1, []byte("zeta")) == -1 {
		t.Fatalf("Marshal output missing alpha/zeta: %s", b1)
	}
	if bytes.Index(b1, []byte("alpha")) > bytes.Index(b1, []byte("zeta")) {
		t.Fatalf("properties should be lexicographical (alpha before zeta): %s", b1)
	}

	b3, _ := schema.Marshal()
	if !bytes.Equal(b1, b3) {
		t.Fatalf("Marshal output non-deterministic:\n%s\nvs\n%s", b1, b3)
	}
}
