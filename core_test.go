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
	if swarm.client == nil {
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
			wantErr:  true,
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

	testFunc := NewAgentFunction(
		"testFunc",
		"Test function description",
		func(args map[string]interface{}) (interface{}, error) {
			return "test result", nil
		},
		[]Parameter{{Name: "name", Type: reflect.TypeOf("string")}},
	)
	errorFunc := NewAgentFunction(
		"errorFunc",
		"Error function description",
		func(args map[string]interface{}) (interface{}, error) {
			return nil, fmt.Errorf("test error")
		},
		[]Parameter{{Name: "name", Type: reflect.TypeOf("string")}},
	)
	agent := NewAgent("TestAgent").
		AddFunction(testFunc).
		AddFunction(errorFunc)

	// Create mock tool calls using our helper
	mockCall := MockToolCall{
		ID:   "test1",
		Name: "testFunc",
		Args: "{}",
	}
	toolCalls := []openai.ChatCompletionMessageToolCall{mockCall.ToOpenAI()}

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

func TestRunAndStream(t *testing.T) {
	swarm := NewSwarm(NewMockOpenAIClient())
	ctx := context.Background()

	agent := NewAgent("TestAgent")
	messages := []map[string]interface{}{
		{
			"role":    "user",
			"content": "Hello",
		},
	}

	stream, err := swarm.RunAndStream(ctx, agent, messages, nil, "", false, 1, true)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	var sawStart, sawEnd, sawContent bool
	for chunk := range stream {
		if delim, ok := chunk["delim"].(string); ok {
			if delim == "start" {
				sawStart = true
			}
			if delim == "end" {
				sawEnd = true
			}
		}
		if _, ok := chunk["content"].(string); ok {
			sawContent = true
		}
	}

	if !sawStart {
		t.Error("Expected to see start delimiter")
	}
	if !sawEnd {
		t.Error("Expected to see end delimiter")
	}
	if !sawContent {
		t.Error("Expected to see content")
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

	swarm := &Swarm{client: client}
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
	swarm := &Swarm{client: mockClient}

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

func TestRunAndStreamWithMockClient(t *testing.T) {
	mockClient := NewMockOpenAIClient()
	swarm := &Swarm{client: mockClient}

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
		1,
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
