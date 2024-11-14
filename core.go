package swarm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openai/openai-go"
)

const ContextVariablesName = "context_variables"

// Swarm orchestrates interactions between agents and OpenAI
type Swarm struct {
	client OpenAIClient
}

// NewSwarm creates a new Swarm instance
func NewSwarm(client OpenAIClient) *Swarm {
	return &Swarm{client: client}
}

// getChatCompletion handles the chat completion request to OpenAI
func (s *Swarm) getChatCompletion(
	ctx context.Context,
	agent *Agent,
	history []map[string]interface{},
	contextVariables map[string]interface{},
	modelOverride string,
	debug bool,
) (*openai.ChatCompletion, error) {
	// Create default context variables if nil
	if contextVariables == nil {
		contextVariables = make(map[string]interface{})
	}

	// Handle instructions with context variables
	var instructions string
	switch i := agent.Instructions.(type) {
	case string:
		instructions = i
	case func(map[string]interface{}) string:
		instructions = i(contextVariables)
	case func() string:
		instructions = i()
	default:
		return nil, fmt.Errorf("invalid instructions type")
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

	paramsJSON, _ := json.Marshal(params)
	DebugPrint(debug, "Getting chat completion for:", string(paramsJSON))

	return s.client.CreateChatCompletion(ctx, params)
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
	// Create default context variables if nil
	if contextVariables == nil {
		contextVariables = make(map[string]interface{})
	}

	functionMap := make(map[string]AgentFunction)
	for _, f := range functions {
		functionMap[f.Name()] = f
	}

	response := &Response{
		Messages:         make([]map[string]interface{}, 0),
		ContextVariables: make(map[string]interface{}),
	}

	for _, toolCall := range toolCalls {
		name := toolCall.Function.Name
		fn, exists := functionMap[name]
		if !exists {
			DebugPrint(debug, fmt.Sprintf("Tool %s not found in function map.", name))
			response.Messages = append(response.Messages, map[string]interface{}{
				"role":         "tool",
				"tool_call_id": toolCall.ID,
				"tool_name":    name,
				"content":      fmt.Sprintf("Error: Tool %s not found.", name),
			})
			continue
		}

		var args map[string]interface{}
		if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
			return nil, fmt.Errorf("failed to parse tool arguments: %v", err)
		}

		DebugPrint(debug, fmt.Sprintf("Processing tool call: %s with arguments %v", name, args))

		// Add context variables to args
		args[ContextVariablesName] = contextVariables

		// Execute function
		rawResult, err := fn.Call(args)
		if err != nil {
			return nil, fmt.Errorf("function execution failed: %v", err)
		}
		DebugPrint(debug, fmt.Sprintf("Tool call result: %q", rawResult))

		result, err := s.handleFunctionResult(rawResult, debug)
		if err != nil {
			return nil, err
		}

		// Update context variables from result
		for k, v := range result.ContextVariables {
			contextVariables[k] = v
			response.ContextVariables[k] = v
		}

		response.Messages = append(response.Messages, map[string]interface{}{
			"role":         "tool",
			"tool_call_id": toolCall.ID,
			"tool_name":    name,
			"content":      result.Value,
		})
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
			messages := prepareMessages(activeAgent.Instructions.(string), history)
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
			stream := s.client.CreateChatCompletionStream(ctx, params)

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
			message := map[string]interface{}{
				"content":    "",
				"sender":     agent.Name,
				"role":       "assistant",
				"tool_calls": make([]map[string]interface{}, 0),
			}
			message["content"] = acc.Choices[0].Message.Content
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
			"content":    completion.Choices[0].Message.Content,
			"sender":     activeAgent.Name,
			"role":       "assistant",
			"tool_calls": completion.Choices[0].Message.ToolCalls,
		}

		DebugPrint(debug, "Received completion:", message)
		history = append(history, message)

		if len(completion.Choices[0].Message.ToolCalls) == 0 || !executeTools {
			DebugPrint(debug, "Ending turn.")
			break
		}

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
