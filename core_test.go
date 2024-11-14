package swarm

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/openai/openai-go"
)

// Mock types for testing
type MockToolCall struct {
	ID   string
	Name string
	Args string
}

func (m MockToolCall) ToOpenAI() openai.ChatCompletionMessageToolCall {
	return openai.ChatCompletionMessageToolCall{
		ID: m.ID,
		Function: openai.ChatCompletionMessageToolCallFunction{
			Name:      m.Name,
			Arguments: m.Args,
		},
		Type: "function",
	}
}

func TestNewSwarm(t *testing.T) {
	swarm := NewSwarm(NewMockOpenAIClient())
	if swarm.Client == nil {
		t.Error("Expected client to be initialized")
	}
}

func TestHandleFunctionResult(t *testing.T) {
	swarm := NewSwarm(NewMockOpenAIClient())
	tests := []struct {
		name     string
		input    interface{}
		expected string
		wantErr  bool
	}{
		{
			name:     "string result",
			input:    "test string",
			expected: "test string",
			wantErr:  false,
		},
		{
			name: "result object",
			input: &Result{
				Value: "test value",
				ContextVariables: map[string]interface{}{
					"test": "value",
				},
			},
			expected: "test value",
			wantErr:  false,
		},
		{
			name:     "agent result",
			input:    NewAgent("TestAgent"),
			expected: `{"assistant":"TestAgent"}`,
			wantErr:  false,
		},
		{
			name:     "nil result",
			input:    nil,
			expected: "",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := swarm.handleFunctionResult(tt.input, false)
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if result.Value != tt.expected {
				t.Errorf("Expected value %q, got %q", tt.expected, result.Value)
			}
		})
	}
}

func TestHandleToolCalls(t *testing.T) {
	swarm := NewSwarm(NewMockOpenAIClient())

	// Create test functions with proper validation
	testFunc := NewAgentFunction(
		"testFunc",
		"Test function description",
		func(args map[string]interface{}) (interface{}, error) {
			return "test result", nil
		},
		[]Parameter{{Name: "name", Type: reflect.TypeOf(""), Description: "Test parameter", Required: true}},
	)
	errorFunc := NewAgentFunction(
		"errorFunc",
		"Error function description",
		func(args map[string]interface{}) (interface{}, error) {
			return nil, fmt.Errorf("test error")
		},
		[]Parameter{{Name: "name", Type: reflect.TypeOf(""), Description: "Test parameter", Required: true}},
	)

	// Create and initialize agent with functions
	agent := NewAgent("TestAgent").
		AddFunction(testFunc).
		AddFunction(errorFunc)

	// Validate agent's functions
	if len(agent.Functions) != 2 {
		t.Fatalf("Expected 2 functions, got %d", len(agent.Functions))
	}

	// Create mock tool calls using our helper
	mockCall := MockToolCall{
		ID:   "test1",
		Name: "testFunc",
		Args: `{"name": "test"}`,
	}
	toolCalls := []openai.ChatCompletionMessageToolCall{mockCall.ToOpenAI()}

	// Pass the agent's functions directly
	response, err := swarm.handleToolCalls(toolCalls, agent.Functions, nil, false)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if len(response.Messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(response.Messages))
	}

	if response.Messages[0]["content"] != "test result" {
		t.Errorf("Expected content 'test result', got %v", response.Messages[0]["content"])
	}
}

func TestRun(t *testing.T) {
	client := NewMockOpenAIClient()
	client.SetCompletionResponse(&openai.ChatCompletion{
		Choices: []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{
					Content: "mock response",
				},
			},
		},
	})

	swarm := &Swarm{Client: client}
	ctx := context.Background()

	agent := NewAgent("TestAgent")
	messages := []map[string]interface{}{
		{
			"role":    "user",
			"content": "Hello",
		},
	}

	response, err := swarm.Run(ctx, agent, messages, nil, "", false, false, 1, true)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if response == nil {
		t.Error("Expected non-nil response")
	}

	if len(response.Messages) == 0 {
		t.Error("Expected at least one message in response")
	}

	if response.Agent == nil {
		t.Error("Expected non-nil agent in response")
	}
}

func TestRunWithMockClient(t *testing.T) {
	mockClient := NewMockOpenAIClient()
	swarm := &Swarm{Client: mockClient}

	// Set up mock response
	mockClient.SetCompletionResponse(&openai.ChatCompletion{
		Choices: []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{
					Content: "Test response",
					Role:    "assistant",
				},
				Index: 0,
			},
		},
	})

	agent := NewAgent("TestAgent")
	messages := []map[string]interface{}{
		{
			"role":    "user",
			"content": "Hello",
		},
	}

	response, err := swarm.Run(
		context.Background(),
		agent,
		messages,
		nil,
		"",
		false,
		false,
		1,
		true,
	)

	AssertNoError(t, err, "Run should not return error")
	AssertEqual(t, "Test response", response.Messages[0]["content"], "Response content should match")
}

func TestRunAndStream(t *testing.T) {
	mockClient := NewMockOpenAIClient()
	swarm := &Swarm{Client: mockClient}

	// Set up mock response
	mockClient.AddStreamChunk(&openai.ChatCompletionChunk{
		Choices: []openai.ChatCompletionChunkChoice{
			{
				Delta: openai.ChatCompletionChunkChoicesDelta{
					Content: "Test",
				},
			},
		},
	})

	agent := NewAgent("TestAgent")
	messages := []map[string]interface{}{
		{
			"role":    "user",
			"content": "Hello",
		},
	}

	stream, err := swarm.RunAndStream(
		context.Background(),
		agent,
		messages,
		nil,
		"",
		false,
		10,
		true,
	)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	var sawContent bool
	for chunk := range stream {
		if content, ok := chunk["content"].(string); ok && content == "Test" {
			sawContent = true
		}
	}

	if !sawContent {
		t.Error("Expected to see content 'Test'")
	}
}

func TestRunAndStreamWithEmptyMessages(t *testing.T) {
	mockClient := NewMockOpenAIClient()
	swarm := &Swarm{Client: mockClient}

	agent := NewAgent("TestAgent")
	messages := []map[string]interface{}{}

	_, err := swarm.RunAndStream(
		context.Background(),
		agent,
		messages,
		nil,
		"",
		false,
		10,
		true,
	)

	if err == nil {
		t.Error("Expected error for empty messages but got none")
	}
}

func TestRunAndStreamWithToolCalls(t *testing.T) {
	mockClient := NewMockOpenAIClient()
	swarm := &Swarm{Client: mockClient}

	// Set up mock response with tool call
	mockClient.AddStreamChunk(&openai.ChatCompletionChunk{
		Choices: []openai.ChatCompletionChunkChoice{
			{
				Delta: openai.ChatCompletionChunkChoicesDelta{
					Content: "Test",
				},
			},
		},
	})
	mockClient.AddStreamChunk(&openai.ChatCompletionChunk{
		Choices: []openai.ChatCompletionChunkChoice{
			{
				Delta: openai.ChatCompletionChunkChoicesDelta{
					FunctionCall: openai.ChatCompletionChunkChoicesDeltaFunctionCall{
						Name:      "testFunc",
						Arguments: "{}",
					},
				},
			},
		},
	})

	agent := NewAgent("TestAgent")
	messages := []map[string]interface{}{
		{
			"role":    "user",
			"content": "Hello",
		},
	}

	stream, err := swarm.RunAndStream(
		context.Background(),
		agent,
		messages,
		nil,
		"",
		false,
		10,
		true,
	)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	var sawContent bool
	var sawToolCall bool
	for chunk := range stream {
		if content, ok := chunk["content"].(string); ok && content == "Test" {
			sawContent = true
		}
		if toolCalls, ok := chunk["tool_calls"].([]map[string]interface{}); ok && len(toolCalls) > 0 {
			sawToolCall = true
		}
	}

	if !sawContent {
		t.Error("Expected to see content 'Test'")
	}

	if !sawToolCall {
		t.Error("Expected to see tool call")
	}
}

func TestRunAndStreamWithAgentTransfer(t *testing.T) {
	mockClient := NewMockOpenAIClient()
	swarm := NewSwarm(mockClient)
	agent1 := NewAgent("Agent1")
	agent2 := NewAgent("Agent2")

	transferFunc := NewAgentFunction(
		"transfer",
		"Transfer to Agent2",
		func(args map[string]interface{}) (interface{}, error) {
			return &Result{
				Value: "Transferring to Agent2...",
				Agent: agent2,
			}, nil
		},
		[]Parameter{},
	)
	agent1.AddFunction(transferFunc)

	mockClient.AddStreamChunk(&openai.ChatCompletionChunk{
		Choices: []openai.ChatCompletionChunkChoice{
			{
				Delta: openai.ChatCompletionChunkChoicesDelta{
					Content: "Test",
				},
			},
		},
	})
	mockClient.AddStreamChunk(&openai.ChatCompletionChunk{
		Choices: []openai.ChatCompletionChunkChoice{
			{
				Delta: openai.ChatCompletionChunkChoicesDelta{
					FunctionCall: openai.ChatCompletionChunkChoicesDeltaFunctionCall{
						Name:      "transfer",
						Arguments: "{}",
					},
				},
			},
		},
	})

	messages := []map[string]interface{}{
		{"role": "user", "content": "Hello"},
	}

	ch, err := swarm.RunAndStream(context.Background(), agent1, messages, nil, "", false, 3, true)
	if err != nil {
		t.Fatalf("RunAndStream failed: %v", err)
	}

	var sawTransfer bool
	for msg := range ch {
		if agent, ok := msg["sender"]; ok && agent == agent2.Name {
			sawTransfer = true
			break
		}
	}

	if !sawTransfer {
		t.Error("Expected to see agent transfer, but didn't")
	}
}

func TestToolPreparationWithContextVariables(t *testing.T) {
	agent := NewAgent("TestAgent")
	testFunc := NewAgentFunction(
		"testFunc",
		"Test function",
		func(args map[string]interface{}) (interface{}, error) {
			return "test", nil
		},
		[]Parameter{
			{Name: "context_variables", Type: reflect.TypeOf(map[string]interface{}{}), Description: "Context variables", Required: true},
			{Name: "param1", Type: reflect.TypeOf(""), Description: "Test parameter", Required: true},
		},
	)
	agent.Functions = append(agent.Functions, testFunc)
	tools := prepareTools(agent)

	// Check that context_variables is not in the tool parameters
	for _, tool := range tools {
		params := tool.Function.Value.Parameters.Value
		if properties, ok := params["properties"].(map[string]interface{}); ok {
			if _, exists := properties["context_variables"]; exists {
				t.Error("context_variables should not be present in tool parameters")
			}
		}
	}
}

func TestMessageAccumulation(t *testing.T) {
	mockClient := NewMockOpenAIClient()
	swarm := NewSwarm(mockClient)

	// Add test chunks to mock client
	mockClient.AddStreamChunk(&openai.ChatCompletionChunk{
		Choices: []openai.ChatCompletionChunkChoice{
			{
				Delta: openai.ChatCompletionChunkChoicesDelta{
					Content: "Test content",
				},
			},
		},
	})
	mockClient.AddStreamChunk(&openai.ChatCompletionChunk{
		Choices: []openai.ChatCompletionChunkChoice{
			{
				Delta: openai.ChatCompletionChunkChoicesDelta{
					FunctionCall: openai.ChatCompletionChunkChoicesDeltaFunctionCall{
						Name:      "testFunc",
						Arguments: "{}",
					},
				},
			},
		},
	})

	agent := NewAgent("TestAgent")
	messages := []map[string]interface{}{
		{"role": "user", "content": "Hello"},
	}

	ch, err := swarm.RunAndStream(context.Background(), agent, messages, nil, "", false, 1, true)
	if err != nil {
		t.Fatalf("RunAndStream failed: %v", err)
	}

	var (
		sawContent  bool
		sawToolCall bool
		sawEnd      bool
	)

	for msg := range ch {
		if content, ok := msg["content"]; ok && content != nil {
			sawContent = true
		}
		if toolCalls, ok := msg["tool_calls"]; ok && toolCalls != nil {
			sawToolCall = true
		}
		if delim, ok := msg["delim"]; ok && delim == "end" {
			sawEnd = true
		}
	}

	if !sawContent {
		t.Error("Expected to see content message")
	}
	if !sawToolCall {
		t.Error("Expected to see tool call message")
	}
	if !sawEnd {
		t.Error("Expected to see end delimiter")
	}
}
