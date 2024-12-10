package swarm

import (
	"context"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Workflow represents a sequence of steps to be executed
// Workflow represents a workflow configuration.
// It contains the following fields:
type Workflow struct {
	// Name is the name of the workflow.
	Name string `yaml:"name" json:"name"`
	// Model specifies the model used in the workflow.
	Model string `yaml:"model" json:"model"`
	// MaxTurns defines the maximum number of turns allowed in the workflow.
	MaxTurns int `yaml:"max_turns" json:"max_turns"`
	// System represents the system prompt for the workflow.
	System string `yaml:"system" json:"system"`
	// Steps is a list of steps involved in the workflow.
	Steps []WorkflowStep `yaml:"steps" json:"steps"`
}

// WorkflowStep represents a single step in a workflow.
type WorkflowStep struct {
	// Name is the name of the workflow step.
	Name string `yaml:"name" json:"name"`
	// Instructions are the instructions for the workflow step.
	Instructions string `yaml:"instructions" json:"instructions"`
	// Inputs are the inputs required for the workflow step.
	Inputs map[string]interface{} `yaml:"inputs" json:"inputs"`

	// Agent is the agent responsible for executing the workflow step.
	Agent *Agent `yaml:"-" json:"-"`
	// Functions are the functions that the agent can perform in this workflow step.
	Functions []AgentFunction `yaml:"-" json:"-"`
}

// Initialize initializes the workflow by setting up agents and their functions
func (w *Workflow) Initialize() {
	if w.MaxTurns == 0 {
		w.MaxTurns = 30
	}

	// Initialize Agent for each step.
	for i := range w.Steps {
		step := &w.Steps[i]
		if step.Agent == nil {
			step.Agent = NewAgent(step.Name)
		}
		if i < len(w.Steps)-1 {
			step.Agent.WithInstructions(fmt.Sprintf("%s\n\nHandoff to the next step after you finish your task.", step.Instructions))
		} else {
			step.Agent.WithInstructions(step.Instructions)
		}
		for _, f := range step.Functions {
			step.Agent.AddFunction(f)
		}
	}

	// Add handoff function for each agent (except the last one).
	for i := range w.Steps {
		step := &w.Steps[i]

		if i < len(w.Steps)-1 {
			nextStep := &w.Steps[i+1]
			handoffFunc := NewAgentFunction(
				fmt.Sprintf("handoffTo%s", nextStep.Name),
				fmt.Sprintf("Handoff to %s step", nextStep.Name),
				func(args map[string]interface{}) (interface{}, error) {
					return &Result{
						Value: fmt.Sprintf("Handoff to %s step...", nextStep.Name),
						Agent: nextStep.Agent,
					}, nil
				},
				[]Parameter{},
			)
			step.Agent.AddFunction(handoffFunc)
		}
	}
}

// LoadWorkflow loads a workflow from a YAML file
func LoadWorkflow(path string) (*Workflow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read workflow file: %w", err)
	}

	var workflow Workflow
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		return nil, fmt.Errorf("failed to unmarshal workflow: %w", err)
	}

	workflow.Initialize()
	return &workflow, nil
}

// Save saves the workflow to a YAML file
func (w *Workflow) Save(path string) error {
	data, err := yaml.Marshal(w)
	if err != nil {
		return fmt.Errorf("failed to marshal workflow: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write workflow file: %w", err)
	}

	return nil
}

// Run executes the workflow steps sequentially
func (w *Workflow) Run(ctx context.Context, client *Swarm) (string, []map[string]interface{}, error) {
	// Context variables to pass between steps
	contextVars := make(map[string]interface{})

	// Start with the first step's agent
	activeAgent := w.Steps[0].Agent

	// Merge initial inputs
	for k, v := range w.Steps[0].Inputs {
		contextVars[k] = v
	}

	// Prepare messages for the inputs
	messages := []map[string]interface{}{
		{
			"role":    "system",
			"content": w.System,
		},
		{
			"role":    "user",
			"content": fmt.Sprintf("Context: %v", contextVars),
		},
	}

	// Execute the step
	response, err := client.Run(ctx, activeAgent, messages, contextVars, w.Model, false, true, w.MaxTurns, true)
	if err != nil {
		return "", nil, err
	}

	return response.Messages[len(response.Messages)-1]["content"].(string), response.Messages, nil
}
