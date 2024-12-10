package swarm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// WorkflowStep represents a single step in a workflow
type WorkflowStep struct {
	Name         string                 `yaml:"name" json:"name"`
	Instructions string                 `yaml:"instructions" json:"instructions"`
	Model        string                 `yaml:"model" json:"model"`
	Inputs       map[string]interface{} `yaml:"inputs" json:"inputs"`

	Functions []AgentFunction `yaml:"-" json:"-"`
	Agent     *Agent          `yaml:"-" json:"-"`
}

// Workflow represents a sequence of steps to be executed
type Workflow struct {
	Name  string         `yaml:"name" json:"name"`
	Steps []WorkflowStep `yaml:"steps" json:"steps"`
}

// StepResult represents the result of a workflow step execution
type StepResult struct {
	StepName string                   `json:"step_name"`
	Messages []map[string]interface{} `json:"messages"`
	Outputs  map[string]interface{}   `json:"outputs"`
	Error    error                    `json:"error,omitempty"`
}

// WorkflowResult represents the result of a workflow execution
type WorkflowResult struct {
	Name    string       `json:"name"`
	Results []StepResult `json:"results"`
}

// LoadFromYAML loads a workflow from a YAML file
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

func (w *Workflow) Initialize() {
	// Initialize agents for each step
	for i := range w.Steps {
		step := &w.Steps[i]
		if step.Agent == nil {
			step.Agent = NewAgent(step.Name)
		}
		if step.Instructions != "" {
			step.Agent.WithInstructions(step.Instructions)
		}
		if step.Model != "" {
			step.Agent.WithModel(step.Model)
		}
		if len(step.Functions) > 0 {
			step.Agent.Functions = step.Functions
		}
	}
}

// SaveToYAML saves the workflow to a YAML file
func (w *Workflow) SaveToYAML(path string) error {
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
func (w *Workflow) Run(ctx context.Context, client *Swarm) (*WorkflowResult, error) {
	result := &WorkflowResult{
		Name:    w.Name,
		Results: make([]StepResult, 0, len(w.Steps)),
	}

	// Context variables to pass between steps
	contextVars := make(map[string]interface{})

	for _, step := range w.Steps {
		// Merge step inputs with context variables
		for k, v := range step.Inputs {
			contextVars[k] = v
		}

		// Prepare messages for the step
		messages := []map[string]interface{}{
			{
				"role":    "system",
				"content": "You are executing as part of a workflow. Process the input and generate appropriate output.",
			},
			{
				"role":    "user",
				"content": fmt.Sprintf("Step: %s\nContext: %v", step.Name, contextVars),
			},
		}

		// Execute the step
		response, err := client.Run(ctx, step.Agent, messages, contextVars, step.Model, false, true, 10, true)
		stepResult := StepResult{
			StepName: step.Name,
			Outputs:  make(map[string]interface{}),
		}
		if err != nil {
			stepResult.Error = err
			result.Results = append(result.Results, stepResult)
			return result, fmt.Errorf("failed to execute step %s: %w", step.Name, err)
		}
		stepResult.Messages = response.Messages

		// Extract outputs from the response messages
		for _, msg := range response.Messages {
			// Try to parse content as JSON first
			if content, ok := msg["content"].(string); ok && content != "" {
				if strings.Contains(content, "```json") {
					re := regexp.MustCompile("(?s)```json\\s*(.*?)\\s*```")
					if matches := re.FindStringSubmatch(content); len(matches) > 1 {
						content = matches[1]
					}
				}
				var outputs map[string]interface{}
				if err := json.Unmarshal([]byte(content), &outputs); err == nil {
					stepResult.Outputs = outputs
					// Update context variables with step outputs
					for k, v := range outputs {
						contextVars[k] = v
					}
					break
				} else {
					stepResult.Outputs["content"] = content
				}
			}

			// Check for function call results
			if role, ok := msg["role"].(string); ok && role == "function" {
				if content, ok := msg["content"].(string); ok {
					var outputs map[string]interface{}
					if err := json.Unmarshal([]byte(content), &outputs); err == nil {
						stepResult.Outputs = outputs
						// Update context variables with step outputs
						for k, v := range outputs {
							contextVars[k] = v
						}
						break
					}
				}
			}
		}

		result.Results = append(result.Results, stepResult)
	}

	return result, nil
}
