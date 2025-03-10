package swarm

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/openai/openai-go"
)

func TestSimpleFlow(t *testing.T) {
	// Create a temporary workflow YAML file
	workflowYAML := `
name: test-workflow
model: gpt-4o
max_turns: 30
system: "You are executing a workflow. Process the input and generate appropriate output."
steps:
  - name: weather-step
    instructions: "You are a weather assistant. Return weather information in JSON format."
    inputs:
      location: "Seattle"
  - name: summary-step
    instructions: "You are a summary assistant. Summarize the weather information in JSON format."
`
	tmpfile, err := os.CreateTemp("", "workflow-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		tmpfile.Close()
		os.Remove(tmpfile.Name())
	}()

	if _, err := tmpfile.Write([]byte(workflowYAML)); err != nil {
		t.Fatal(err)
	}

	// Load workflow from YAML
	workflow, err := LoadSimpleFlow(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to load workflow: %v", err)
	}

	// Add functions to steps
	weatherFunc := NewAgentFunction(
		"getWeather",
		"Get weather information",
		func(args map[string]interface{}) (interface{}, error) {
			return map[string]interface{}{
				"temperature": 72,
				"condition":   "sunny",
			}, nil
		},
		[]Parameter{},
	)
	workflow.Steps[0].Functions = append(workflow.Steps[0].Functions, weatherFunc)

	// Create mock client with expected responses
	mockClient := NewMockOpenAIClient()

	// First step: weather info with function call and result
	mockClient.SetCompletionResponse(&openai.ChatCompletion{
		Choices: []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{
					Role:    "assistant",
					Content: "",
					ToolCalls: []openai.ChatCompletionMessageToolCall{
						{
							Function: openai.ChatCompletionMessageToolCallFunction{
								Name:      "getWeather",
								Arguments: `{"location": "Seattle"}`,
							},
						},
					},
				},
			},
		},
	})

	// Add function result to mock client's response
	mockClient.SetCompletionResponse(&openai.ChatCompletion{
		Choices: []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{
					Role:    "assistant",
					Content: `{"temperature": 72, "condition": "sunny"}`,
				},
			},
		},
	})

	// Second step: summary
	mockClient.SetCompletionResponse(&openai.ChatCompletion{
		Choices: []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{
					Content: `{"summary": "The weather is warm and sunny", "recommendation": "Great day for outdoor activities"}`,
					Role:    "assistant",
				},
			},
		},
	})

	client := NewSwarm(mockClient)

	// Run workflow
	result, _, err := workflow.Run(context.Background(), client)
	if err != nil {
		t.Fatalf("Failed to run workflow: %v", err)
	}

	// Verify results
	var summaryResult map[string]interface{}
	if err := json.Unmarshal([]byte(result), &summaryResult); err != nil {
		t.Fatalf("Failed to unmarshal result: %v, raw result: %s", err, result)
	}

	// Verify summary step result
	expectedSummary := map[string]interface{}{
		"summary":        "The weather is warm and sunny",
		"recommendation": "Great day for outdoor activities",
	}

	for k, v := range expectedSummary {
		got, ok := summaryResult[k]
		if !ok {
			t.Errorf("Missing expected key %s in summary result", k)
			continue
		}
		if got != v {
			t.Errorf("Expected %s=%v, got %v", k, v, got)
		}
	}
}

func TestSimpleFlowSaveLoad(t *testing.T) {
	workflow := &SimpleFlow{
		Name:     "test-workflow",
		Model:    "gpt-4o",
		MaxTurns: 30,
		System:   "You are executing a workflow. Process the input and generate appropriate output.",
		Steps: []SimpleFlowStep{
			{
				Name:         "step1",
				Instructions: "Test instructions",
				Inputs: map[string]interface{}{
					"key": "value",
				},
			},
		},
	}

	// Save workflow
	tmpfile, err := os.CreateTemp("", "workflow-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		tmpfile.Close()
		os.Remove(tmpfile.Name())
	}()

	if err := workflow.Save(tmpfile.Name()); err != nil {
		t.Fatalf("Failed to save workflow: %v", err)
	}

	// Load workflow
	loaded, err := LoadSimpleFlow(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to load workflow: %v", err)
	}

	// Verify loaded workflow
	if loaded.Name != workflow.Name {
		t.Errorf("Expected workflow name %s, got %s", workflow.Name, loaded.Name)
	}
	if len(loaded.Steps) != len(workflow.Steps) {
		t.Errorf("Expected %d steps, got %d", len(workflow.Steps), len(loaded.Steps))
	}
	if loaded.Steps[0].Name != workflow.Steps[0].Name {
		t.Errorf("Expected step name %s, got %s", workflow.Steps[0].Name, loaded.Steps[0].Name)
	}
	if loaded.Steps[0].Instructions != workflow.Steps[0].Instructions {
		t.Errorf("Expected instructions %s, got %s", workflow.Steps[0].Instructions, loaded.Steps[0].Instructions)
	}
	if v, ok := loaded.Steps[0].Inputs["key"]; !ok || v != "value" {
		t.Errorf("Expected input key=value, got %v", loaded.Steps[0].Inputs["key"])
	}
}
