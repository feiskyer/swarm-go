package swarm

import (
	"reflect"
	"testing"
)

func TestNewAgent(t *testing.T) {
	agent := NewAgent("TestAgent")

	if agent.Name != "TestAgent" {
		t.Errorf("Expected agent name to be TestAgent, got %s", agent.Name)
	}

	if agent.Model != "gpt-4" {
		t.Errorf("Expected default model to be gpt-4, got %s", agent.Model)
	}

	if agent.Instructions != "You are a helpful agent." {
		t.Errorf("Expected default instructions, got %v", agent.Instructions)
	}

	if len(agent.Functions) != 0 {
		t.Errorf("Expected empty functions slice, got %d functions", len(agent.Functions))
	}

	if !agent.ParallelToolCalls {
		t.Error("Expected ParallelToolCalls to be true by default")
	}
}

func TestAgentChaining(t *testing.T) {
	testFunc := func(args map[string]interface{}) (interface{}, error) {
		return "test", nil
	}

	agent := NewAgent("TestAgent").
		WithModel("gpt-4").
		WithInstructions("Custom instructions").
		AddFunction(NewAgentFunction(
			"testFunc",
			"Test function description",
			testFunc,
			[]Parameter{{Name: "name", Type: reflect.TypeOf("string")}},
		))

	if agent.Model != "gpt-4" {
		t.Errorf("Expected model to be gpt-4, got %s", agent.Model)
	}

	if agent.Instructions != "Custom instructions" {
		t.Errorf("Expected custom instructions, got %v", agent.Instructions)
	}

	if len(agent.Functions) != 1 {
		t.Errorf("Expected 1 function, got %d", len(agent.Functions))
	}
}

func TestResult(t *testing.T) {
	agent := NewAgent("TestAgent")
	result := &Result{
		Value: "test value",
		Agent: agent,
		ContextVariables: map[string]interface{}{
			"key": "value",
		},
	}

	if result.Value != "test value" {
		t.Errorf("Expected value to be 'test value', got %s", result.Value)
	}

	if result.Agent != agent {
		t.Error("Expected agent reference to match")
	}

	if v, ok := result.ContextVariables["key"]; !ok || v != "value" {
		t.Errorf("Expected context variable 'key' to be 'value', got %v", v)
	}
}
