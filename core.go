// Package swarm provides functionality for orchestrating interactions between agents and OpenAI's language models.
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
	// ErrEmptyMessages indicates that the messages array is empty
	ErrEmptyMessages = errors.New("messages cannot be empty")
	// ErrInvalidToolCall indicates an invalid tool call
	ErrInvalidToolCall = errors.New("invalid tool call")
)

// ContextVariablesName is the name of the key used to store context variables.
const ContextVariablesName = "context_variables"

// Swarm orchestrates interactions between agents and OpenAI.
type Swarm struct {
	Client OpenAIClient
}

// NewSwarm creates a new Swarm instance with the provided OpenAI client.
func NewSwarm(client OpenAIClient) *Swarm {
	if client == nil {
		panic("OpenAI client cannot be nil")
	}
	return &Swarm{Client: client}
}

// NewDefaultSwarm creates a new Swarm instance with the default OpenAI client inferred from the environment variables.
// It will use OpenAI if OPENAI_API_KEY is set, otherwise it will use Azure OpenAI if AZURE_OPENAI_API_KEY and AZURE_OPENAI_API_BASE are set.
func NewDefaultSwarm() (*Swarm, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey != "" {
		return NewSwarm(NewOpenAIClient(apiKey)), nil
	}

	azureApiKey := os.Getenv("AZURE_OPENAI_API_KEY")
	azureApiBase := os.Getenv("AZURE_OPENAI_API_BASE")

	var missingEnvs []string
	if azureApiKey == "" {
		missingEnvs = append(missingEnvs, "AZURE_OPENAI_API_KEY")
	}
	if azureApiBase == "" {
		missingEnvs = append(missingEnvs, "AZURE_OPENAI_API_BASE")
	}

	if len(missingEnvs) > 0 {
		return nil, fmt.Errorf("required environment variables not set: %s", strings.Join(missingEnvs, ", "))
	}

	return NewSwarm(NewAzureOpenAIClient(azureApiKey, azureApiBase)), nil
}

// getChatCompletion handles the chat completion request to OpenAI.
func (s *Swarm) getChatCompletion(
	ctx context.Context,
	agent *Agent,
	history []map[string]interface{},
	contextVariables map[string]interface{},
	modelOverride string,
	debug bool,
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
	messages := prepareMessages(instructions, history)

	// Prepare tools
	tools := prepareTools(agent)

	// Create completion parameters
	params := openai.ChatCompletionNewParams{
		Messages: openai.F(messages),
		Model:    openai.F(modelOverride),
	}
	if len(tools) > 0 {
		params.Tools = openai.F(tools)
		if agent.ToolChoice != nil {
			params.ToolChoice = openai.F(*agent.ToolChoice)
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
				Type: openai.F(openai.ChatCompletionToolTypeFunction),
				Function: openai.F(openai.FunctionDefinitionParam{
					Name:        openai.String(funcJSON["function"].(map[string]interface{})["name"].(string)),
					Description: openai.String(funcJSON["function"].(map[string]interface{})["description"].(string)),
					Parameters: openai.F(openai.FunctionParameters{
						"type":       "object",
						"properties": funcJSON["function"].(map[string]interface{})["parameters"].(map[string]interface{})["properties"].(map[string]interface{}),
						"required":   funcJSON["function"].(map[string]interface{})["parameters"].(map[string]interface{})["required"].([]string),
					}),
				}),
			})
		}
	}
	return tools
}

func prepareMessages(instructions string, history []map[string]interface{}) []openai.ChatCompletionMessageParamUnion {
	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(instructions),
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
			messages = append(messages, openai.FunctionMessage(name, content))
		case "tool":
			toolCallID, _ := msg["tool_call_id"].(string)
			messages = append(messages, openai.ToolMessage(toolCallID, content))
		default:
			assistantMsg := openai.ChatCompletionAssistantMessageParam{
				Role: openai.F(openai.ChatCompletionAssistantMessageParamRoleAssistant),
				Content: openai.F([]openai.ChatCompletionAssistantMessageParamContentUnion{
					openai.ChatCompletionAssistantMessageParamContent{
						Type: openai.F(openai.ChatCompletionAssistantMessageParamContentTypeText),
						Text: openai.F(content),
					},
				}),
			}
			if toolCalls, ok := msg["tool_calls"].([]openai.ChatCompletionMessageToolCall); ok {
				toolCallParams := make([]openai.ChatCompletionMessageToolCallParam, len(toolCalls))
				for i, tc := range toolCalls {
					toolCallParams[i] = openai.ChatCompletionMessageToolCallParam{
						ID:   openai.String(tc.ID),
						Type: openai.F(tc.Type),
						Function: openai.F(openai.ChatCompletionMessageToolCallFunctionParam{
							Name:      openai.String(tc.Function.Name),
							Arguments: openai.String(tc.Function.Arguments),
						}),
					}
				}
				assistantMsg.ToolCalls = openai.F(toolCallParams)
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

// RunAndStream executes the agent interaction with streaming responses
func (s *Swarm) RunAndStream(
	ctx context.Context,
	agent *Agent,
	messages []map[string]interface{},
	contextVariables map[string]interface{},
	modelOverride string,
	debug bool,
	maxTurns int,
	executeTools bool,
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

			messages := prepareMessages(instructions, history)
			params := openai.ChatCompletionNewParams{
				Messages: openai.F(messages),
				Model:    openai.F(modelOverride),
			}
			if len(tools) > 0 {
				params.Tools = openai.F(tools)
				if agent.ToolChoice != nil {
					params.ToolChoice = openai.F(*agent.ToolChoice)
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

// Run executes the agent interaction without streaming
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
) (*Response, error) {
	if stream {
		ch, err := s.RunAndStream(ctx, agent, messages, contextVariables, modelOverride, debug, maxTurns, executeTools)
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
		completion, err := s.getChatCompletion(ctx, activeAgent, history, contextVariables, modelOverride, debug)
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
