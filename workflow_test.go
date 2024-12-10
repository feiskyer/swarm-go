package swarm

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/openai/openai-go"
)

func TestWorkflow(t *testing.T) {
	// Create a temporary workflow YAML file
	workflowYAML := `
name: test-workflow
steps:
  - name: weather-step
    instructions: "You are a weather assistant. Return weather information in JSON format."
    model: gpt-4o
    inputs:
      location: "Seattle"
  - name: summary-step
    instructions: "You are a summary assistant. Summarize the weather information in JSON format."
    model: gpt-4o
`
	tmpfile, err := os.CreateTemp("", "workflow-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(workflowYAML)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	// Load workflow from YAML
	workflow, err := LoadWorkflow(tmpfile.Name())
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
	workflow.Steps[0].Functions = []AgentFunction{weatherFunc}

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
	result, err := workflow.Run(context.Background(), client)
	if err != nil {
		t.Fatalf("Failed to run workflow: %v", err)
	}

	// Verify results
	if len(result.Results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(result.Results))
	}

	// Verify step names
	if result.Results[0].StepName != "weather-step" {
		t.Errorf("Expected first step name to be weather-step, got %s", result.Results[0].StepName)
	}
	if result.Results[1].StepName != "summary-step" {
		t.Errorf("Expected second step name to be summary-step, got %s", result.Results[1].StepName)
	}

	// Verify outputs
	weatherOutput := result.Results[0].Outputs
	fmt.Println(weatherOutput)
	if temp, ok := weatherOutput["temperature"].(float64); !ok || temp != 72 {
		t.Errorf("Expected temperature 72, got %v", weatherOutput["temperature"])
	}
	if cond, ok := weatherOutput["condition"].(string); !ok || cond != "sunny" {
		t.Errorf("Expected condition sunny, got %v", weatherOutput["condition"])
	}

	summaryOutput := result.Results[1].Outputs
	if summary, ok := summaryOutput["summary"].(string); !ok || summary != "The weather is warm and sunny" {
		t.Errorf("Expected summary 'The weather is warm and sunny', got %v", summaryOutput["summary"])
	}
}

func TestWorkflowSaveLoad(t *testing.T) {
	workflow := &Workflow{
		Name: "test-workflow",
		Steps: []WorkflowStep{
			{
				Name:         "step1",
				Instructions: "Test instructions",
				Model:        "gpt-4o",
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
	defer os.Remove(tmpfile.Name())

	if err := workflow.SaveToYAML(tmpfile.Name()); err != nil {
		t.Fatalf("Failed to save workflow: %v", err)
	}

	// Load workflow
	loaded, err := LoadWorkflow(tmpfile.Name())
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
	if loaded.Steps[0].Model != workflow.Steps[0].Model {
		t.Errorf("Expected model %s, got %s", workflow.Steps[0].Model, loaded.Steps[0].Model)
	}
	if v, ok := loaded.Steps[0].Inputs["key"]; !ok || v != "value" {
		t.Errorf("Expected input key=value, got %v", loaded.Steps[0].Inputs["key"])
	}
}
