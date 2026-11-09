//

//

//

package tools

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

type SchemaType string

const (
	TypeString  SchemaType = "string"
	TypeNumber  SchemaType = "number"
	TypeInteger SchemaType = "integer"
	TypeBoolean SchemaType = "boolean"
	TypeArray   SchemaType = "array"
	TypeObject  SchemaType = "object"
	TypeNull    SchemaType = "null"
)

type JsonSchemaNode struct {
	Type SchemaType `json:"type,omitempty"`

	Description string `json:"description,omitempty"`

	Properties map[string]*JsonSchemaNode `json:"properties,omitempty"`

	Required []string `json:"required,omitempty"`

	AdditionalProperties *bool `json:"additionalProperties,omitempty"`

	Items *JsonSchemaNode `json:"items,omitempty"`

	Enum []any `json:"enum,omitempty"`

	Const any `json:"const,omitempty"`

	Default any `json:"default,omitempty"`

	Examples []any `json:"examples,omitempty"`

	OneOf []*JsonSchemaNode `json:"oneOf,omitempty"`

	Minimum          *float64 `json:"minimum,omitempty"`
	Maximum          *float64 `json:"maximum,omitempty"`
	ExclusiveMinimum *bool    `json:"exclusiveMinimum,omitempty"`
	ExclusiveMaximum *bool    `json:"exclusiveMaximum,omitempty"`

	MinLength *int   `json:"minLength,omitempty"`
	MaxLength *int   `json:"maxLength,omitempty"`
	Pattern   string `json:"pattern,omitempty"`

	MinItems *int `json:"minItems,omitempty"`
	MaxItems *int `json:"maxItems,omitempty"`
}

var supportedKeys = map[string]struct{}{
	"type": {}, "description": {}, "properties": {}, "required": {},
	"additionalProperties": {}, "items": {}, "enum": {}, "const": {},
	"default": {}, "examples": {}, "oneOf": {},
	"minimum": {}, "maximum": {}, "exclusiveMinimum": {}, "exclusiveMaximum": {},
	"minLength": {}, "maxLength": {}, "pattern": {},
	"minItems": {}, "maxItems": {},
}

func Compile(raw []byte) (*JsonSchemaNode, error) {
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()

	var node JsonSchemaNode
	if err := dec.Decode(&node); err != nil {
		return nil, fmt.Errorf("schema: compile failed: %w", err)
	}

	if err := validateSupportedKeys(raw); err != nil {
		return nil, err
	}

	if err := node.assertStructural(); err != nil {
		return nil, err
	}
	return &node, nil
}

func validateSupportedKeys(raw []byte) error {
	var root any
	if err := json.Unmarshal(raw, &root); err != nil {
		return fmt.Errorf("schema: invalid json: %w", err)
	}
	var walk func(v any) error
	walk = func(v any) error {
		switch t := v.(type) {
		case map[string]any:
			for k, sub := range t {
				if _, ok := supportedKeys[k]; !ok {
					return fmt.Errorf("schema: unsupported keyword %q (fail-closed, keyword not supported)", k)
				}
				if k == "properties" {

					if props, ok := sub.(map[string]any); ok {
						for _, subSchema := range props {
							if err := walk(subSchema); err != nil {
								return err
							}
						}
					}
					continue
				}
				if err := walk(sub); err != nil {
					return err
				}
			}
		case []any:
			for _, item := range t {
				if err := walk(item); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walk(root)
}

func (n *JsonSchemaNode) assertStructural() error {
	if n.OneOf != nil && len(n.OneOf) == 0 {
		return fmt.Errorf("schema: oneOf must contain at least one subschema")
	}
	if n.Type == TypeObject && n.Properties == nil && n.AdditionalProperties == nil {
		return fmt.Errorf("schema: object type must declare properties or additionalProperties=false")
	}
	if n.Type == TypeArray && n.Items == nil {
		return fmt.Errorf("schema: array type must declare items")
	}
	for _, name := range n.Required {
		if _, ok := n.Properties[name]; !ok && n.Type == TypeObject {
			return fmt.Errorf("schema: required property %q not declared in properties", name)
		}
	}
	return nil
}

func (n *JsonSchemaNode) Validate(v any) error {

	if len(n.OneOf) > 0 {
		var matched int
		for _, sub := range n.OneOf {
			if err := sub.Validate(v); err == nil {
				matched++
			}
		}
		if matched == 0 {
			return fmt.Errorf("schema: value %v does not match any oneOf subschema", v)
		}
		return nil
	}

	if len(n.Enum) > 0 {
		for _, e := range n.Enum {
			if reflect.DeepEqual(e, v) {
				return n.validateTyped(v)
			}
		}
		return fmt.Errorf("schema: value %v not in enum %v", v, n.Enum)
	}

	if n.Const != nil {
		if !reflect.DeepEqual(n.Const, v) {
			return fmt.Errorf("schema: value %v != const %v", v, n.Const)
		}
		return n.validateTyped(v)
	}

	return n.validateTyped(v)
}

func (n *JsonSchemaNode) validateTyped(v any) error {
	switch n.Type {
	case "":

		return nil
	case TypeNull:
		if v != nil {
			return fmt.Errorf("schema: expected null, got %v", v)
		}
		return nil
	case TypeString:
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("schema: expected string, got %T", v)
		}
		if n.MinLength != nil && len([]rune(s)) < *n.MinLength {
			return fmt.Errorf("schema: string length %d < minLength %d", len([]rune(s)), *n.MinLength)
		}
		if n.MaxLength != nil && len([]rune(s)) > *n.MaxLength {
			return fmt.Errorf("schema: string length %d > maxLength %d", len([]rune(s)), *n.MaxLength)
		}
		if n.Pattern != "" {
			re, err := regexp.Compile(n.Pattern)
			if err != nil {
				return fmt.Errorf("schema: invalid pattern %q: %w", n.Pattern, err)
			}
			if !re.MatchString(s) {
				return fmt.Errorf("schema: string %q does not match pattern %q", s, n.Pattern)
			}
		}
		return nil
	case TypeNumber, TypeInteger:
		f, ok := toFloat(v)
		if !ok {
			return fmt.Errorf("schema: expected number, got %T", v)
		}
		if n.Type == TypeInteger {
			if math.Trunc(f) != f {
				return fmt.Errorf("schema: expected integer, got %v", v)
			}
		}
		if n.Minimum != nil {
			if n.ExclusiveMinimum != nil && *n.ExclusiveMinimum && f <= *n.Minimum {
				return fmt.Errorf("schema: value %v must be > %v", f, *n.Minimum)
			}
			if f < *n.Minimum {
				return fmt.Errorf("schema: value %v < minimum %v", f, *n.Minimum)
			}
		}
		if n.Maximum != nil {
			if n.ExclusiveMaximum != nil && *n.ExclusiveMaximum && f >= *n.Maximum {
				return fmt.Errorf("schema: value %v must be < %v", f, *n.Maximum)
			}
			if f > *n.Maximum {
				return fmt.Errorf("schema: value %v > maximum %v", f, *n.Maximum)
			}
		}
		return nil
	case TypeBoolean:
		if _, ok := v.(bool); !ok {
			return fmt.Errorf("schema: expected boolean, got %T", v)
		}
		return nil
	case TypeArray:
		arr, ok := v.([]any)
		if !ok {

			rv := reflect.ValueOf(v)
			if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
				return fmt.Errorf("schema: expected array, got %T", v)
			}
			arr = make([]any, rv.Len())
			for i := 0; i < rv.Len(); i++ {
				arr[i] = rv.Index(i).Interface()
			}
		}
		if n.MinItems != nil && len(arr) < *n.MinItems {
			return fmt.Errorf("schema: array length %d < minItems %d", len(arr), *n.MinItems)
		}
		if n.MaxItems != nil && len(arr) > *n.MaxItems {
			return fmt.Errorf("schema: array length %d > maxItems %d", len(arr), *n.MaxItems)
		}
		if n.Items != nil {
			for i, item := range arr {
				if err := n.Items.Validate(item); err != nil {
					return fmt.Errorf("schema: items[%d]: %w", i, err)
				}
			}
		}
		return nil
	case TypeObject:
		obj, ok := v.(map[string]any)
		if !ok {

			obj = structToMap(v)
			if obj == nil {
				return fmt.Errorf("schema: expected object, got %T", v)
			}
		}

		for _, name := range n.Required {
			if _, exists := obj[name]; !exists {
				return fmt.Errorf("schema: missing required property %q", name)
			}
		}

		if n.AdditionalProperties != nil && !*n.AdditionalProperties {
			for key := range obj {
				if _, declared := n.Properties[key]; !declared {
					return fmt.Errorf("schema: unexpected property %q (additionalProperties=false)", key)
				}
			}
		}

		for key, sub := range n.Properties {
			if val, exists := obj[key]; exists {
				if err := sub.Validate(val); err != nil {
					return fmt.Errorf("schema: property %q: %w", key, err)
				}
			}
		}
		return nil
	default:
		return fmt.Errorf("schema: unsupported type %q", n.Type)
	}
}

func toFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case int32:
		return float64(t), true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

func structToMap(v any) map[string]any {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil
	}
	out := map[string]any{}
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		name := field.Name
		if tag, ok := field.Tag.Lookup("json"); ok {
			if parts := strings.Split(tag, ","); len(parts) > 0 && parts[0] != "" {
				name = parts[0]
			}
		}
		out[name] = rv.Field(i).Interface()
	}
	return out
}

func Infer(v any) (*JsonSchemaNode, error) {
	node, err := infer(reflect.ValueOf(v), 0)
	if err != nil {
		return nil, err
	}
	return node, nil
}

func infer(rv reflect.Value, depth int) (*JsonSchemaNode, error) {
	if depth > 32 {
		return nil, fmt.Errorf("schema: inference depth exceeded (possible recursive type)")
	}
	if !rv.IsValid() {
		return &JsonSchemaNode{Type: TypeNull}, nil
	}
	if rv.Kind() == reflect.Ptr || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return &JsonSchemaNode{Type: TypeNull}, nil
		}
		return infer(rv.Elem(), depth+1)
	}

	switch rv.Kind() {
	case reflect.String:
		return &JsonSchemaNode{Type: TypeString}, nil
	case reflect.Bool:
		return &JsonSchemaNode{Type: TypeBoolean}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &JsonSchemaNode{Type: TypeInteger}, nil
	case reflect.Float32, reflect.Float64:
		return &JsonSchemaNode{Type: TypeNumber}, nil
	case reflect.Slice, reflect.Array:

		items := &JsonSchemaNode{Type: TypeNull}
		if rv.Kind() == reflect.Array && rv.Len() > 0 {
			it, err := infer(rv.Index(0), depth+1)
			if err != nil {
				return nil, err
			}
			items = it
		} else if rv.Kind() == reflect.Slice {
			elemType := rv.Type().Elem()
			it, err := infer(reflect.New(elemType).Elem(), depth+1)
			if err != nil {
				return nil, err
			}
			items = it
		}
		return &JsonSchemaNode{Type: TypeArray, Items: items}, nil
	case reflect.Map:
		props := map[string]*JsonSchemaNode{}
		iter := rv.MapRange()
		for iter.Next() {
			key := iter.Key()
			if key.Kind() != reflect.String {
				return nil, fmt.Errorf("schema: cannot infer schema from non-string map key %v", key)
			}
			sub, err := infer(iter.Value(), depth+1)
			if err != nil {
				return nil, err
			}
			props[key.String()] = sub
		}
		return &JsonSchemaNode{Type: TypeObject, Properties: props}, nil
	case reflect.Struct:
		props := map[string]*JsonSchemaNode{}
		var required []string
		rt := rv.Type()
		for i := 0; i < rt.NumField(); i++ {
			field := rt.Field(i)
			if !field.IsExported() {
				continue
			}
			name := field.Name
			optional := false
			if tag, ok := field.Tag.Lookup("json"); ok {
				parts := strings.Split(tag, ",")
				if len(parts) > 0 && parts[0] != "" {
					name = parts[0]
				}
				for _, p := range parts[1:] {
					if p == "omitempty" {
						optional = true
					}
				}
			}
			sub, err := infer(rv.Field(i), depth+1)
			if err != nil {
				return nil, err
			}
			props[name] = sub
			if !optional {
				required = append(required, name)
			}
		}
		return &JsonSchemaNode{Type: TypeObject, Properties: props, Required: required}, nil
	default:
		return nil, fmt.Errorf("schema: cannot infer schema from kind %v", rv.Kind())
	}
}

func (n *JsonSchemaNode) Marshal() ([]byte, error) {

	ordered := n.orderMap()
	return json.MarshalIndent(ordered, "", "  ")
}

func (n *JsonSchemaNode) orderMap() map[string]any {
	out := map[string]any{}
	if n.Type != "" {
		out["type"] = n.Type
	}
	if n.Description != "" {
		out["description"] = n.Description
	}
	if len(n.Properties) > 0 {
		props := map[string]any{}
		keys := make([]string, 0, len(n.Properties))
		for k := range n.Properties {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			props[k] = n.Properties[k].orderMap()
		}
		out["properties"] = props
	}
	if len(n.Required) > 0 {
		req := append([]string(nil), n.Required...)
		sort.Strings(req)
		out["required"] = req
	}
	if n.AdditionalProperties != nil {
		out["additionalProperties"] = *n.AdditionalProperties
	}
	if n.Items != nil {
		out["items"] = n.Items.orderMap()
	}
	if len(n.Enum) > 0 {
		out["enum"] = n.Enum
	}
	if n.Const != nil {
		out["const"] = n.Const
	}
	if n.Default != nil {
		out["default"] = n.Default
	}
	if len(n.Examples) > 0 {
		out["examples"] = n.Examples
	}
	if len(n.OneOf) > 0 {
		oneOf := make([]any, 0, len(n.OneOf))
		for _, sub := range n.OneOf {
			oneOf = append(oneOf, sub.orderMap())
		}
		out["oneOf"] = oneOf
	}
	if n.Minimum != nil {
		out["minimum"] = *n.Minimum
	}
	if n.Maximum != nil {
		out["maximum"] = *n.Maximum
	}
	if n.ExclusiveMinimum != nil {
		out["exclusiveMinimum"] = *n.ExclusiveMinimum
	}
	if n.ExclusiveMaximum != nil {
		out["exclusiveMaximum"] = *n.ExclusiveMaximum
	}
	if n.MinLength != nil {
		out["minLength"] = *n.MinLength
	}
	if n.MaxLength != nil {
		out["maxLength"] = *n.MaxLength
	}
	if n.Pattern != "" {
		out["pattern"] = n.Pattern
	}
	if n.MinItems != nil {
		out["minItems"] = *n.MinItems
	}
	if n.MaxItems != nil {
		out["maxItems"] = *n.MaxItems
	}
	return out
}
