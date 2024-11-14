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

// Add these new types
type ToolCall struct {
	Function Function `json:"function"`
}

type Function struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// processAndPrintStreamingResponse handles streaming response processing and printing
func processAndPrintStreamingResponse(responseChan <-chan map[string]interface{}) *Response {
	var content string
	var lastSender string

	for chunk := range responseChan {
		resp := StreamResponse{}
		// Convert map to struct for type safety
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
// Json marshal/unmarshal not used here because funtion pointers are not supported.
// e.g. 'json: unsupported type: func(map[string]interface {}) (interface {}, error)'
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

	var client *Swarm
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		azureApiKey := os.Getenv("AZURE_OPENAI_API_KEY")
		if azureApiKey == "" {
			fmt.Println("OPENAI_API_KEY or AZURE_OPENAI_API_KEY is not set")
			return
		}

		azureApiBase := os.Getenv("AZURE_OPENAI_API_BASE")
		if azureApiBase == "" {
			fmt.Println("AZURE_OPENAI_API_BASE is not set")
			return
		}
		client = NewSwarm(NewAzureOpenAIClient(azureApiKey, azureApiBase))
	} else {
		client = NewSwarm(NewOpenAIClient(apiKey))
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