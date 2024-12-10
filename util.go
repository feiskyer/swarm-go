package swarm

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"time"
)

const (
	colorBlue   = "\033[94m"
	colorPurple = "\033[95m"
	colorGray   = "\033[90m"
	colorReset  = "\033[0m"
)

// StreamResponse represents a streaming response chunk
type StreamResponse struct {
	Content   string     `json:"content,omitempty"`
	Sender    string     `json:"sender,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Delim     string     `json:"delim,omitempty"`
	Response  *Response  `json:"response,omitempty"`
}

// ToolCall represents a call to a specific tool/function.
type ToolCall struct {
	Function Function `json:"function"`
}

// Function represents a function call, including its name and arguments.
type Function struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

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
	if f == nil {
		return nil
	}

	params := f.Parameters()
	if params == nil {
		return map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        f.Name(),
				"description": f.Description(),
				"parameters": map[string]interface{}{
					"type": "object",
				},
			},
		}
	}

	properties := make(map[string]interface{})
	required := []string{}

	for i := 0; i < len(params); i++ {
		paramName := params[i].Name
		paramType := params[i].Type

		if paramType == nil {
			// If type is not specified, default to string
			properties[paramName] = map[string]interface{}{
				"type":        "string",
				"description": params[i].Description,
			}
		} else if paramType.Kind() == reflect.Struct {
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
	if t == nil {
		return "string"
	}

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

// processAndPrintStreamingResponse handles streaming response processing and printing
func processAndPrintStreamingResponse(responseChan <-chan map[string]interface{}) *Response {
	var content string
	var lastSender string

	for chunk := range responseChan {
		resp := StreamResponse{}
		if err := mapToStruct(chunk, &resp); err != nil {
			fmt.Printf("Error processing chunk: %v\n", err)
			continue
		}

		if resp.Sender != "" {
			lastSender = resp.Sender
		}

		if resp.Content != "" {
			if content == "" && lastSender != "" {
				fmt.Printf("%s%s:%s ", colorBlue, lastSender, colorReset)
				lastSender = ""
			}
			fmt.Print(resp.Content)
			content += resp.Content
		}

		if len(resp.ToolCalls) > 0 {
			for _, toolCall := range resp.ToolCalls {
				if toolCall.Function.Name != "" {
					fmt.Printf("%s%s: %s%s%s()\n",
						colorBlue, lastSender,
						colorPurple, toolCall.Function.Name,
						colorReset)
				}
			}
		}

		if resp.Delim == "end" && content != "" {
			fmt.Println()
			content = ""
		}

		if resp.Response != nil {
			return resp.Response
		}
	}

	return nil
}

// mapToStruct safely converts a map to a struct
// Json marshal/unmarshal not used here because of error:
// 'json: unsupported type: func(map[string]interface {}) (interface {}, error)'
func mapToStruct(m map[string]interface{}, v interface{}) error {
	// Get the reflected value and ensure it's a pointer to a struct
	val := reflect.ValueOf(v)
	if val.Kind() != reflect.Ptr || val.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("v must be a pointer to a struct")
	}

	structVal := val.Elem()
	structType := structVal.Type()

	// Iterate through struct fields
	for i := 0; i < structVal.NumField(); i++ {
		field := structVal.Field(i)
		fieldType := structType.Field(i)

		// Get the json tag name
		jsonTag := strings.Split(fieldType.Tag.Get("json"), ",")[0]
		if jsonTag == "-" {
			continue
		}
		if jsonTag == "" {
			jsonTag = fieldType.Name
		}

		// Get the value from the map
		if value, ok := m[jsonTag]; ok && value != nil {
			// Handle the value based on field type
			if err := setField(field, reflect.ValueOf(value)); err != nil {
				return fmt.Errorf("failed to set field %s: %v", fieldType.Name, err)
			}
		}
	}
	return nil
}

func setField(field reflect.Value, value reflect.Value) error {
	if !field.CanSet() {
		return fmt.Errorf("field cannot be set")
	}

	// Handle type conversions
	switch field.Kind() {
	case reflect.Slice:
		if value.Kind() == reflect.Slice {
			slice := reflect.MakeSlice(field.Type(), value.Len(), value.Cap())
			for i := 0; i < value.Len(); i++ {
				if err := setField(slice.Index(i), value.Index(i)); err != nil {
					return err
				}
			}
			field.Set(slice)
		}
	case reflect.Struct:
		if value.Kind() == reflect.Map {
			// Recursively handle nested structs
			if err := mapToStruct(value.Interface().(map[string]interface{}), field.Addr().Interface()); err != nil {
				return err
			}
		}
	default:
		if field.Type().AssignableTo(value.Type()) {
			field.Set(value)
		} else if value.Type().ConvertibleTo(field.Type()) {
			field.Set(value.Convert(field.Type()))
		} else {
			return fmt.Errorf("cannot assign %v to %v", value.Type(), field.Type())
		}
	}
	return nil
}

// prettyPrintMessages prints messages with color formatting
func prettyPrintMessages(messages []map[string]interface{}) {
	for _, message := range messages {
		if role, ok := message["role"].(string); !ok || role != "assistant" {
			continue
		}

		sender := message["sender"].(string)
		fmt.Printf("%s%s%s:", colorBlue, sender, colorReset)

		if content, ok := message["content"].(string); ok && content != "" {
			fmt.Printf(" %s\n", content)
		}

		if toolCalls, ok := message["tool_calls"].([]map[string]interface{}); ok && len(toolCalls) > 0 {
			if len(toolCalls) > 1 {
				fmt.Println()
			}
			for _, toolCall := range toolCalls {
				if function, ok := toolCall["function"].(map[string]interface{}); ok {
					name := function["name"].(string)
					args := function["arguments"].(string)

					var argsMap map[string]interface{}
					if err := json.Unmarshal([]byte(args), &argsMap); err != nil {
						fmt.Printf("Error parsing arguments: %v\n", err)
						continue
					}

					fmt.Printf("%s%s%s(%s)\n",
						colorPurple, name, colorReset,
						formatArgs(argsMap))
				}
			}
		}
	}
}

// formatArgs formats argument map to string
func formatArgs(args map[string]interface{}) string {
	pairs := make([]string, 0, len(args))
	for k, v := range args {
		pairs = append(pairs, fmt.Sprintf("%s=%v", k, v))
	}
	return strings.Join(pairs, ", ")
}

// RunDemoLoop starts an interactive CLI session
func RunDemoLoop(startingAgent *Agent, contextVariables map[string]interface{}, stream bool, debug bool) {
	fmt.Println("Starting Swarm CLI 🐝")

	client, err := NewDefaultSwarm()
	if err != nil {
		fmt.Printf("Error creating Swarm client: %v\n", err)
		return
	}

	messages := make([]map[string]interface{}, 0)
	agent := startingAgent

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("%sUser%s: ", colorGray, colorReset)
		input, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Println("Exiting Swarm CLI 🐝")
				return
			}

			fmt.Printf("Error reading input: %v\n", err)
			continue
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		messages = append(messages, map[string]interface{}{
			"role":    "user",
			"content": input,
		})

		ctx := context.Background()
		if stream {
			responseChan, err := client.RunAndStream(ctx, agent, messages, contextVariables, "gpt-4o", debug, 10, true)
			if err != nil {
				fmt.Printf("Error in stream: %v\n", err)
				continue
			}

			response := processAndPrintStreamingResponse(responseChan)
			if response != nil {
				messages = append(messages, response.Messages...)
				agent = response.Agent
			}
		} else {
			response, err := client.Run(ctx, agent, messages, contextVariables, "gpt-4o", false, debug, 10, true)
			if err != nil {
				fmt.Printf("Error in run: %v\n", err)
				continue
			}

			prettyPrintMessages(response.Messages)
			messages = append(messages, response.Messages...)
			agent = response.Agent
		}
	}
}
