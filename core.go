package swarm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/openai/openai-go"
)

var (
	// ErrEmptyMessages indicates that the messages array is empty when making a request.
	// This error is returned when attempting to run an agent interaction without any initial messages.
	ErrEmptyMessages = errors.New("messages cannot be empty")

	// ErrInvalidToolCall indicates that a tool call request was malformed or invalid.
	// This can occur when the tool call parameters don't match the function signature.
	ErrInvalidToolCall = errors.New("invalid tool call")
)

// ContextVariablesName is the key used to store context variables in function arguments.
// This constant is used internally to pass context between function calls.
const ContextVariablesName = "context_variables"

// Swarm orchestrates interactions between agents and OpenAI's language models.
// It handles message processing, tool execution, and response management.
type Swarm struct {
	// Client is the interface to OpenAI's API
	Client OpenAIClient
}

// NewSwarm creates a new Swarm instance with the provided OpenAI client.
//
// Parameters:
//   - client: An implementation of OpenAIClient interface for API communication
//
// Returns:
//   - *Swarm: A new Swarm instance
func NewSwarm(client OpenAIClient) *Swarm {
	if client == nil {
		panic("OpenAI client cannot be nil")
	}
	return &Swarm{Client: client}
}

// NewDefaultSwarm creates a new Swarm instance with default OpenAI client configuration.
// It uses the OPENAI_API_KEY environment variable for authentication.
// Returns an error if the API key is not set or if client creation fails.
func NewDefaultSwarm() (*Swarm, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey != "" {
		apiBase := os.Getenv("OPENAI_API_BASE")
		if apiBase == "" {
			return NewSwarm(NewOpenAIClient(apiKey)), nil
		}

		return NewSwarm(NewOpenAIClientWithBaseURL(apiKey, apiBase)), nil
	}

	azureAPIKey := os.Getenv("AZURE_OPENAI_API_KEY")
	azureAPIBase := os.Getenv("AZURE_OPENAI_API_BASE")
	azureAPIVersion := os.Getenv("AZURE_OPENAI_API_VERSION")

	var missingEnvs []string
	if azureAPIKey == "" {
		missingEnvs = append(missingEnvs, "AZURE_OPENAI_API_KEY")
	}
	if azureAPIBase == "" {
		missingEnvs = append(missingEnvs, "AZURE_OPENAI_API_BASE")
	}
	if azureAPIVersion == "" {
		azureAPIVersion = "2025-03-01-preview"
	}

	if len(missingEnvs) > 0 {
		return nil, fmt.Errorf("required environment variables not set: %s", strings.Join(missingEnvs, ", "))
	}

	return NewSwarm(NewAzureOpenAIClient(azureAPIKey, azureAPIBase, azureAPIVersion)), nil
}

// getChatCompletion sends a request to OpenAI's chat completion API and returns the response.
// It handles message preparation, tool configuration, and response parsing.
//
// Parameters:
//   - ctx: Context for the request
//   - agent: Agent configuration including tools and instructions
//   - history: Previous conversation messages
//   - contextVariables: Variables to be used in the conversation
//   - modelOverride: Optional model override (uses agent's default if empty)
//   - debug: Enable debug logging
//
// Returns the chat completion response or an error if the request fails.
func (s *Swarm) getChatCompletion(
	ctx context.Context,
	agent *Agent,
	history []map[string]interface{},
	contextVariables map[string]interface{},
	modelOverride string,
	debug bool,
	jsonMode bool,
) (*openai.ChatCompletion, error) {
	if agent == nil {
		return nil, errors.New("agent cannot be nil")
	}

	if contextVariables == nil {
		contextVariables = make(map[string]interface{})
	}

	instructions, err := s.getInstructions(agent, contextVariables)
	if err != nil {
		return nil, err
	}

	// Prepare messages
	model := modelOverride
	if model == "" {
		model = agent.Model
	}
	messages := prepareMessages(instructions, history, model)

	// Prepare tools
	tools := prepareTools(agent)

	// Create completion parameters
	params := openai.ChatCompletionNewParams{
		Messages: messages,
		Model:    openai.ChatModel(modelOverride),
	}
	if jsonMode {
		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &openai.ResponseFormatJSONObjectParam{},
		}
	}
	if len(tools) > 0 {
		params.Tools = tools
		if agent.ToolChoice != nil {
			params.ToolChoice = *agent.ToolChoice
		}
	}

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal params: %w", err)
	}
	DebugPrint(debug, "Getting chat completion for:", string(paramsJSON))

	return s.Client.CreateChatCompletion(ctx, params)
}

// getInstructions safely extracts instructions from the agent based on its type.
func (s *Swarm) getInstructions(agent *Agent, contextVariables map[string]interface{}) (string, error) {
	switch i := agent.Instructions.(type) {
	case string:
		return i, nil
	case func(map[string]interface{}) string:
		return i(contextVariables), nil
	case func() string:
		return i(), nil
	default:
		return "", ErrInvalidInstruction
	}
}

func prepareTools(agent *Agent) []openai.ChatCompletionToolParam {
	var tools []openai.ChatCompletionToolParam
	for _, f := range agent.Functions {
		funcJSON := FunctionToJSON(f)
		if funcJSON != nil {

			if params, ok := funcJSON["function"].(map[string]interface{})["parameters"].(map[string]interface{}); ok {
				if props, ok := params["properties"].(map[string]interface{}); ok {
					delete(props, ContextVariablesName)
				}
			}

			tools = append(tools, openai.ChatCompletionToolParam{
				Function: openai.FunctionDefinitionParam{
					Name:        funcJSON["function"].(map[string]interface{})["name"].(string),
					Description: openai.String(funcJSON["function"].(map[string]interface{})["description"].(string)),
					Parameters: openai.FunctionParameters{
						"type":       "object",
						"properties": funcJSON["function"].(map[string]interface{})["parameters"].(map[string]interface{})["properties"].(map[string]interface{}),
						"required":   funcJSON["function"].(map[string]interface{})["parameters"].(map[string]interface{})["required"].([]string),
					},
				},
			})
		}
	}
	return tools
}

func prepareMessages(instructions string, history []map[string]interface{}, model string) []openai.ChatCompletionMessageParamUnion {
	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(instructions),
	}
	if strings.Contains(model, "o1") || strings.Contains(model, "o3") || strings.Contains(strings.ToLower(model), "deekseek") {
		messages = []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(instructions),
		}
	}

	for _, msg := range history {
		content, _ := msg["content"].(string)
		role, _ := msg["role"].(string)

		switch role {
		case "user":
			messages = append(messages, openai.UserMessage(content))
		case "system":

		case "function":
			name, _ := msg["name"].(string)
			messages = append(messages, openai.ToolMessage(content, name))
		case "tool":
			toolCallID, _ := msg["tool_call_id"].(string)
			messages = append(messages, openai.ToolMessage(content, toolCallID))
		default:
			assistantMsg := openai.AssistantMessage(content)
			if toolCalls, ok := msg["tool_calls"].([]openai.ChatCompletionMessageToolCall); ok {
				toolCallParams := make([]openai.ChatCompletionMessageToolCallParam, len(toolCalls))
				for i, tc := range toolCalls {
					toolCallParams[i] = openai.ChatCompletionMessageToolCallParam{
						ID:   tc.ID,
						Type: tc.Type,
						Function: openai.ChatCompletionMessageToolCallFunctionParam{
							Name:      tc.Function.Name,
							Arguments: tc.Function.Arguments,
						},
					}
				}
				assistantMsg.OfAssistant.ToolCalls = toolCallParams
			}
			messages = append(messages, assistantMsg)
		}
	}
	return messages
}

// handleFunctionResult processes the result from an agent function
func (s *Swarm) handleFunctionResult(result interface{}, debug bool) (*Result, error) {
	if result == nil {
		return &Result{}, nil
	}

	switch v := result.(type) {
	case *Result:
		return v, nil
	case *Agent:
		return &Result{
			Value: fmt.Sprintf(`{"assistant":"%s"}`, v.Name),
			Agent: v,
		}, nil
	default:
		str := fmt.Sprintf("%v", v)
		if str == "" {
			err := fmt.Errorf("failed to cast response to string: %v", result)
			DebugPrint(debug, err.Error())
			return nil, err
		}
		return &Result{Value: str}, nil
	}
}

// handleToolCalls processes tool calls from the chat completion
func (s *Swarm) handleToolCalls(
	toolCalls []openai.ChatCompletionMessageToolCall,
	functions []AgentFunction,
	contextVariables map[string]interface{},
	debug bool,
) (*Response, error) {
	if len(toolCalls) == 0 {
		return nil, fmt.Errorf("no tool calls provided")
	}

	if functions == nil {
		return nil, fmt.Errorf("functions cannot be nil")
	}

	// Create default context variables if nil
	if contextVariables == nil {
		contextVariables = make(map[string]interface{})
	}

	functionMap := make(map[string]AgentFunction, len(functions))
	for _, f := range functions {
		if f != nil {
			functionMap[f.Name()] = f
		}
	}

	response := &Response{
		Messages:         make([]map[string]interface{}, 0, len(toolCalls)),
		ContextVariables: make(map[string]interface{}, len(contextVariables)),
	}

	// Copy initial context variables
	for k, v := range contextVariables {
		response.ContextVariables[k] = v
	}

	for _, toolCall := range toolCalls {
		name := toolCall.Function.Name
		fn, exists := functionMap[name]
		if !exists {
			errMsg := fmt.Sprintf("Tool %q not found in function map", name)
			DebugPrint(debug, errMsg)
			response.Messages = append(response.Messages, map[string]interface{}{
				"role":         "tool",
				"tool_call_id": toolCall.ID,
				"tool_name":    name,
				"content":      fmt.Sprintf("Error: %s", errMsg),
			})
			continue
		}

		var args map[string]interface{}
		if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
			errMsg := fmt.Sprintf("Failed to parse arguments for tool %q: %v", name, err)
			DebugPrint(debug, errMsg)
			response.Messages = append(response.Messages, map[string]interface{}{
				"role":         "tool",
				"tool_call_id": toolCall.ID,
				"tool_name":    name,
				"content":      fmt.Sprintf("Error: %s", errMsg),
			})
			continue
		}

		// Add context variables to args
		args[ContextVariablesName] = contextVariables

		// Execute function
		rawResult, err := fn.Call(args)
		if err != nil {
			errMsg := fmt.Sprintf("Function %q execution failed: %v", name, err)
			DebugPrint(debug, errMsg)
			response.Messages = append(response.Messages, map[string]interface{}{
				"role":         "tool",
				"tool_call_id": toolCall.ID,
				"tool_name":    name,
				"content":      fmt.Sprintf("Error: %s", errMsg),
			})
			continue
		}

		result, err := s.handleFunctionResult(rawResult, debug)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to handle result for tool %q: %v", name, err)
			DebugPrint(debug, errMsg)
			response.Messages = append(response.Messages, map[string]interface{}{
				"role":         "tool",
				"tool_call_id": toolCall.ID,
				"tool_name":    name,
				"content":      fmt.Sprintf("Error: %s", errMsg),
			})
			continue
		}

		// Update context variables from result
		for k, v := range result.ContextVariables {
			contextVariables[k] = v
			response.ContextVariables[k] = v
		}

		// Update agent if transferred
		if result.Agent != nil {
			response.Agent = result.Agent
		}

		// Create tool response message
		message := map[string]interface{}{
			"role":         "tool",
			"tool_call_id": toolCall.ID,
			"tool_name":    name,
			"content":      result.Value,
		}

		// Add agent name if agent transfer occurred
		if result.Agent != nil {
			message["agent"] = result.Agent.Name
		}

		response.Messages = append(response.Messages, message)
	}

	return response, nil
}

// RunAndStream executes an interaction with the OpenAI model and returns a channel
// that streams the response tokens as they arrive.
//
// Parameters:
//   - ctx: Context for the request
//   - agent: Agent configuration including tools and instructions
//   - messages: Conversation history
//   - contextVariables: Variables to be used in the conversation
//   - modelOverride: Optional model override (uses agent's default if empty)
//   - debug: Enable debug logging
//   - maxTurns: Maximum number of interaction turns
//   - executeTools: Whether to execute tool calls
//
// Returns a channel of response tokens or an error if the streaming setup fails.
func (s *Swarm) RunAndStream(
	ctx context.Context,
	agent *Agent,
	messages []map[string]interface{},
	contextVariables map[string]interface{},
	modelOverride string,
	debug bool,
	maxTurns int,
	executeTools bool,
	jsonMode bool,
) (<-chan map[string]interface{}, error) {
	if len(messages) == 0 {
		return nil, ErrEmptyMessages
	}

	if agent == nil {
		return nil, errors.New("agent cannot be nil")
	}

	if contextVariables == nil {
		contextVariables = make(map[string]interface{})
	}

	resultChan := make(chan map[string]interface{})
	activeAgent := agent
	history := make([]map[string]interface{}, len(messages))
	copy(history, messages)
	initLen := len(messages)

	// Prepare tools
	tools := prepareTools(agent)

	go func() {
		defer close(resultChan)

		for len(history)-initLen < maxTurns {
			instructions, err := s.getInstructions(activeAgent, contextVariables)
			if err != nil {
				DebugPrint(debug, "Failed to get instructions:", err)
				return
			}
			model := modelOverride
			if model == "" {
				model = activeAgent.Model
			}
			messages := prepareMessages(instructions, history, model)
			params := openai.ChatCompletionNewParams{
				Messages: messages,
				Model:    modelOverride,
			}
			if jsonMode {
				params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
					OfJSONObject: &openai.ResponseFormatJSONObjectParam{},
				}
			}
			if len(tools) > 0 {
				params.Tools = tools
				if agent.ToolChoice != nil {
					params.ToolChoice = *agent.ToolChoice
				}
			}
			stream, err := s.Client.CreateChatCompletionStream(ctx, params)
			if err != nil {
				DebugPrint(debug, "Failed to create chat completion stream:", err)
				return
			}

			resultChan <- map[string]interface{}{"delim": "start"}
			acc := openai.ChatCompletionAccumulator{}
			for stream.Next() {
				chunk := stream.Current()
				acc.AddChunk(chunk)

				if content, ok := acc.JustFinishedContent(); ok {
					resultChan <- map[string]interface{}{
						"content": content,
						"sender":  activeAgent.Name,
					}
				}

				if tool, ok := acc.JustFinishedToolCall(); ok {
					resultChan <- map[string]interface{}{
						"tool_calls": []map[string]interface{}{
							{
								"id": tool.Index,
								"function": map[string]interface{}{
									"name":      tool.Name,
									"arguments": tool.Arguments,
								},
							},
						},
					}
				}
			}

			resultChan <- map[string]interface{}{"delim": "end"}

			if err := stream.Err(); err != nil {
				DebugPrint(debug, "Stream error:", err)
				return
			}

			// Process accumulated response
			if len(acc.Choices) == 0 {
				DebugPrint(debug, "No choices in the response.")
				return
			}

			message := map[string]interface{}{
				"content":    acc.Choices[0].Message.Content,
				"sender":     activeAgent.Name,
				"role":       "assistant",
				"tool_calls": make([]map[string]interface{}, 0),
			}
			if len(acc.Choices[0].Message.ToolCalls) > 0 {
				message["tool_calls"] = acc.Choices[0].Message.ToolCalls
			}

			DebugPrint(debug, "Received completion:", message)
			history = append(history, message)

			toolCalls := acc.Choices[0].Message.ToolCalls
			if len(toolCalls) == 0 || !executeTools {
				DebugPrint(debug, "Ending turn.")
				break
			}

			// Handle tool calls
			response, err := s.handleToolCalls(toolCalls, activeAgent.Functions, contextVariables, debug)
			if err != nil {
				DebugPrint(debug, "Tool call error:", err)
				return
			}

			history = append(history, response.Messages...)
			for k, v := range response.ContextVariables {
				contextVariables[k] = v
			}
			if response.Agent != nil {
				activeAgent = response.Agent
			}
		}

		// Send final response
		resultChan <- map[string]interface{}{
			"response": &Response{
				Messages:         history[initLen:],
				Agent:            activeAgent,
				ContextVariables: contextVariables,
			},
		}
	}()

	return resultChan, nil
}

// Run executes a single interaction with the OpenAI model using the provided agent configuration.
// It supports both streaming and non-streaming modes, tool execution, and debug logging.
//
// Parameters:
//   - ctx: Context for the request
//   - agent: Agent configuration including tools and instructions
//   - messages: Conversation history
//   - contextVariables: Variables to be used in the conversation
//   - modelOverride: Optional model override (uses agent's default if empty)
//   - stream: Enable streaming mode
//   - debug: Enable debug logging
//   - maxTurns: Maximum number of interaction turns
//   - executeTools: Whether to execute tool calls
//
// Returns a Response containing the model's output and any tool execution results,
// or an error if the interaction fails.
func (s *Swarm) Run(
	ctx context.Context,
	agent *Agent,
	messages []map[string]interface{},
	contextVariables map[string]interface{},
	modelOverride string,
	stream bool,
	debug bool,
	maxTurns int,
	executeTools bool,
	jsonMode bool,
) (*Response, error) {
	if stream {
		ch, err := s.RunAndStream(ctx, agent, messages, contextVariables, modelOverride, debug, maxTurns, executeTools, false)
		if err != nil {
			return nil, err
		}

		var finalResponse *Response
		for msg := range ch {
			if resp, ok := msg["response"]; ok {
				if r, ok := resp.(*Response); ok {
					finalResponse = r
				}
			}
		}
		return finalResponse, nil
	}

	if contextVariables == nil {
		contextVariables = make(map[string]interface{})
	}

	activeAgent := agent
	history := make([]map[string]interface{}, len(messages))
	copy(history, messages)
	initLen := len(messages)

	for len(history)-initLen < maxTurns {
		completion, err := s.getChatCompletion(ctx, activeAgent, history, contextVariables, modelOverride, debug, jsonMode)
		if err != nil {
			return nil, err
		}

		message := map[string]interface{}{
			"content": completion.Choices[0].Message.Content,
			"sender":  activeAgent.Name,
			"role":    "assistant",
		}
		if len(completion.Choices[0].Message.ToolCalls) > 0 {
			message["tool_calls"] = completion.Choices[0].Message.ToolCalls
		}

		DebugPrint(debug, "Received completion:", message)
		history = append(history, message)

		if len(completion.Choices[0].Message.ToolCalls) == 0 || !executeTools {
			DebugPrint(debug, "Ending turn.")
			break
		}

		// Handle tool calls
		response, err := s.handleToolCalls(completion.Choices[0].Message.ToolCalls, activeAgent.Functions, contextVariables, debug)
		if err != nil {
			return nil, err
		}

		history = append(history, response.Messages...)
		for k, v := range response.ContextVariables {
			contextVariables[k] = v
		}
		if response.Agent != nil {
			activeAgent = response.Agent
		}
	}

	return &Response{
		Messages:         history[initLen:],
		Agent:            activeAgent,
		ContextVariables: contextVariables,
	}, nil
}
