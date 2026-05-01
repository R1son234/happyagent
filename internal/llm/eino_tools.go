package llm

import (
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/schema"
)

type jsonSchemaObject struct {
	Type       string                        `json:"type"`
	Properties map[string]jsonSchemaProperty `json:"properties"`
	Required   []string                      `json:"required"`
}

type jsonSchemaProperty struct {
	Type        string                        `json:"type"`
	Description string                        `json:"description"`
	Enum        []string                      `json:"enum"`
	Properties  map[string]jsonSchemaProperty `json:"properties"`
	Items       *jsonSchemaProperty           `json:"items"`
}

func toEinoToolInfos(specs []ToolSpec) ([]*schema.ToolInfo, error) {
	infos := make([]*schema.ToolInfo, 0, len(specs))
	for _, spec := range specs {
		params, err := parseToolParams(spec.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("tool %q: %w", spec.Name, err)
		}

		infos = append(infos, &schema.ToolInfo{
			Name:        spec.Name,
			Desc:        spec.Description,
			ParamsOneOf: schema.NewParamsOneOfByParams(params),
		})
	}
	return infos, nil
}

func parseToolParams(raw string) (map[string]*schema.ParameterInfo, error) {
	if raw == "" {
		return map[string]*schema.ParameterInfo{}, nil
	}

	var root jsonSchemaObject
	if err := json.Unmarshal([]byte(raw), &root); err != nil {
		return nil, fmt.Errorf("parse input schema JSON: %w", err)
	}

	if root.Type != "" && root.Type != string(schema.Object) {
		return nil, fmt.Errorf("top-level input schema must be object, got %q", root.Type)
	}

	required := make(map[string]struct{}, len(root.Required))
	for _, name := range root.Required {
		required[name] = struct{}{}
	}

	params := make(map[string]*schema.ParameterInfo, len(root.Properties))
	for name, property := range root.Properties {
		param, err := toParameterInfo(property)
		if err != nil {
			return nil, fmt.Errorf("property %q: %w", name, err)
		}
		_, isRequired := required[name]
		param.Required = isRequired
		params[name] = param
	}

	return params, nil
}

func toParameterInfo(property jsonSchemaProperty) (*schema.ParameterInfo, error) {
	dataType, err := toDataType(property.Type)
	if err != nil {
		return nil, err
	}

	param := &schema.ParameterInfo{
		Type: dataType,
		Desc: property.Description,
		Enum: property.Enum,
	}

	switch dataType {
	case schema.Array:
		if property.Items == nil {
			return nil, fmt.Errorf("array schema is missing items")
		}
		elem, err := toParameterInfo(*property.Items)
		if err != nil {
			return nil, fmt.Errorf("array items: %w", err)
		}
		param.ElemInfo = elem
	case schema.Object:
		if len(property.Properties) == 0 {
			param.SubParams = map[string]*schema.ParameterInfo{}
			return param, nil
		}

		subParams := make(map[string]*schema.ParameterInfo, len(property.Properties))
		for name, sub := range property.Properties {
			info, err := toParameterInfo(sub)
			if err != nil {
				return nil, fmt.Errorf("object field %q: %w", name, err)
			}
			subParams[name] = info
		}
		param.SubParams = subParams
	}

	return param, nil
}

func toDataType(value string) (schema.DataType, error) {
	switch value {
	case "string":
		return schema.String, nil
	case "number":
		return schema.Number, nil
	case "integer":
		return schema.Integer, nil
	case "boolean":
		return schema.Boolean, nil
	case "array":
		return schema.Array, nil
	case "object", "":
		return schema.Object, nil
	case "null":
		return schema.Null, nil
	default:
		return "", fmt.Errorf("unsupported JSON schema type %q", value)
	}
}
