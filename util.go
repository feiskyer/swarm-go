package swarm

import (
	"fmt"
	"reflect"
	"time"
)

// DebugPrint prints debug information if debug is enabled
func DebugPrint(debug bool, args ...interface{}) {
	if !debug {
		return
	}
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprint(args...)
	fmt.Printf("\033[97m[\033[90m%s\033[97m]\033[90m %s\033[0m\n", timestamp, message)
}

// FunctionToJSON converts a Go function to OpenAI function format
func FunctionToJSON(f AgentFunction) map[string]interface{} {
	properties := make(map[string]interface{})
	required := make([]string, 0)

	for i := 0; i < len(f.Parameters()); i++ {
		paramType := f.Parameters()[i].Type
		paramName := f.Parameters()[i].Name

		// If the type is a struct, try to get field names
		if paramType.Kind() == reflect.Struct {
			structProperties := make(map[string]interface{})
			for j := 0; j < paramType.NumField(); j++ {
				field := paramType.Field(j)
				structProperties[field.Name] = map[string]interface{}{
					"type": getJSONType(field.Type),
				}
			}
			properties[paramName] = map[string]interface{}{
				"type":       "object",
				"properties": structProperties,
			}
		} else {
			properties[paramName] = map[string]interface{}{
				"type": getJSONType(paramType),
			}
		}
		required = append(required, paramName)
	}

	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        f.Name(),
			"description": f.Description(),
			"parameters": map[string]interface{}{
				"type":       "object",
				"properties": properties,
				"required":   required,
			},
		},
	}
}

// MergeFields merges source fields into target map recursively
func MergeFields(target, source map[string]interface{}) {
	for key, value := range source {
		if targetValue, exists := target[key]; exists {
			if mapValue, ok := value.(map[string]interface{}); ok {
				if targetMap, ok := targetValue.(map[string]interface{}); ok {
					MergeFields(targetMap, mapValue)
					continue
				}
			}
		}
		target[key] = value
	}
}

// getJSONType converts Go types to JSON schema types
func getJSONType(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "integer"
	case reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Bool:
		return "boolean"
	case reflect.Slice, reflect.Array:
		return "array"
	case reflect.Map, reflect.Struct:
		return "object"
	case reflect.Interface:
		return "object" // Handle interface{} as generic object
	default:
		return "string" // Default to string for unknown types
	}
}
