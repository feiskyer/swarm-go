package swarm

import (
	"context"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// SimpleFlow represents a sequential workflow that executes a series of steps
// using AI agents. Each step in the workflow is executed in order, with the ability
// to pass context between steps and manage timeouts.
type SimpleFlow struct {
	// Name is the name of the workflow.
	Name string `yaml:"name" json:"name"`
	// Model specifies the model used in the workflow.
	Model string `yaml:"model" json:"model"`
	// MaxTurns defines the maximum number of turns allowed in the workflow.
	MaxTurns int `yaml:"max_turns" json:"max_turns"`
	// System represents the system prompt for the workflow.
	System string `yaml:"system" json:"system"`
	// Steps is a list of steps involved in the workflow.
	Steps []SimpleFlowStep `yaml:"steps" json:"steps"`
	// Verbose specifies whether to print verbose logs.
	Verbose bool `yaml:"verbose" json:"verbose"`
	// Timeout specifies the timeout for the entire workflow.
	Timeout time.Duration `yaml:"timeout" json:"timeout"`
}

// SimpleFlowStep defines a single step within a SimpleFlow workflow. Each step
// represents an atomic operation performed by an AI agent with specific instructions
// and capabilities.
type SimpleFlowStep struct {
	// Name is the name of the workflow step.
	Name string `yaml:"name" json:"name"`
	// Instructions are the instructions for the workflow step.
	Instructions string `yaml:"instructions" json:"instructions"`
	// Inputs are the inputs required for the workflow step.
	Inputs map[string]interface{} `yaml:"inputs" json:"inputs"`
	// Timeout specifies the timeout for this step. If not set, uses workflow timeout.
	Timeout time.Duration `yaml:"timeout" json:"timeout"`

	// Agent is the agent responsible for executing the workflow step.
	Agent *Agent `yaml:"-" json:"-"`
	// Functions are the functions that the agent can perform in this workflow step.
	Functions []AgentFunction `yaml:"-" json:"-"`
}

// SimpleStepResult contains the output and metadata from executing a workflow step.
type SimpleStepResult struct {
	StepName string
	Content  string
	Messages []map[string]interface{}
	Error    error
}

// Initialize prepares the workflow for execution by setting up default values,
// configuring agents, and establishing connections between steps. It must be
// called before running the workflow.
//
// The function performs the following setup:
//   - Sets default values for MaxTurns and Timeout if not specified
//   - Validates the workflow has at least one step
//   - Initializes agents for each step
//   - Configures step-specific timeouts
//   - Sets up handoff functions between consecutive steps
//
// Returns an error if the workflow configuration is invalid.
func (w *SimpleFlow) Initialize() error {
	if w.MaxTurns == 0 {
		w.MaxTurns = 30
	}
	if w.Timeout == 0 {
		w.Timeout = 5 * time.Minute
	}

	if len(w.Steps) == 0 {
		return fmt.Errorf("workflow must have at least one step")
	}

	// Initialize Agent for each step.
	for i := range w.Steps {
		step := &w.Steps[i]
		if step.Agent == nil {
			step.Agent = NewAgent(step.Name)
		}
		if step.Timeout == 0 {
			step.Timeout = w.Timeout / time.Duration(len(w.Steps))
		}

		// Add step instructions
		if i < len(w.Steps)-1 {
			step.Agent.WithInstructions(fmt.Sprintf("%s\n\nHandoff to the next step after you finish your task.", step.Instructions))
		} else {
			step.Agent.WithInstructions(step.Instructions)
		}

		// Add step functions
		for _, f := range step.Functions {
			step.Agent.AddFunction(f)
		}

		// Add handoff function if not last step
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

	return nil
}

// LoadSimpleFlow creates a new SimpleFlow instance from a YAML configuration file.
// The function reads the file, unmarshals the YAML content, and initializes the
// workflow.
func LoadSimpleFlow(path string) (*SimpleFlow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read workflow file: %w", err)
	}

	var workflow SimpleFlow
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		return nil, fmt.Errorf("failed to unmarshal workflow: %w", err)
	}

	if err := workflow.Initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize workflow: %w", err)
	}

	return &workflow, nil
}

// Save persists the workflow configuration to a YAML file at the specified path.
// The function marshals the workflow structure to YAML format and writes it to
// the filesystem.
func (w *SimpleFlow) Save(path string) error {
	data, err := yaml.Marshal(w)
	if err != nil {
		return fmt.Errorf("failed to marshal workflow: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write workflow file: %w", err)
	}

	return nil
}

// executeStep runs a single step of the workflow with the provided context and
// variables. It handles timeout management, input merging, and message preparation.
//
// Parameters:
//   - ctx: Context for timeout and cancellation
//   - client: The Swarm client for executing AI operations
//   - step: The workflow step to execute
//   - contextVars: Variables passed from previous steps
//   - prevMessages: Conversation history from previous steps
//
// Returns the step execution result and any error encountered.
func (w *SimpleFlow) executeStep(ctx context.Context, client *Swarm, step *SimpleFlowStep, contextVars map[string]interface{}, prevMessages []map[string]interface{}) (*SimpleStepResult, error) {
	// Create step context with timeout
	stepCtx, cancel := context.WithTimeout(ctx, step.Timeout)
	defer cancel()

	// Validate step configuration
	if step.Agent == nil {
		return nil, fmt.Errorf("step %s has no agent configured", step.Name)
	}

	// Merge step inputs with context vars
	mergedVars := make(map[string]interface{}, len(contextVars)+len(step.Inputs))
	for k, v := range contextVars {
		mergedVars[k] = v
	}
	for k, v := range step.Inputs {
		mergedVars[k] = v
	}

	// Prepare messages
	messages := make([]map[string]interface{}, 0, len(prevMessages)+2)
	messages = append(messages, map[string]interface{}{
		"role":    "system",
		"content": w.System,
	})
	messages = append(messages, prevMessages...)
	messages = append(messages, map[string]interface{}{
		"role":    "user",
		"content": fmt.Sprintf("Context: %v", mergedVars),
	})

	// Execute step with error handling
	response, err := client.Run(stepCtx, step.Agent, messages, mergedVars, w.Model, false, w.Verbose, w.MaxTurns, true)
	if err != nil {
		return &SimpleStepResult{
			StepName: step.Name,
			Error:    fmt.Errorf("step %s execution failed: %w", step.Name, err),
		}, err
	}

	// Validate response
	if response == nil || len(response.Messages) == 0 {
		return nil, fmt.Errorf("step %s returned no response", step.Name)
	}

	// Extract result
	content := response.Messages[len(response.Messages)-1]["content"].(string)
	return &SimpleStepResult{
		StepName: step.Name,
		Content:  content,
		Messages: response.Messages,
	}, nil
}

// Run executes all steps in the workflow sequentially, managing timeouts and
// passing context between steps. It initializes the workflow if needed and
// handles any errors that occur during execution.
//
// Parameters:
//   - ctx: Context for timeout and cancellation
//   - client: The Swarm client for executing AI operations
//
// Returns:
//   - string: The content from the last step
//   - []map[string]interface{}: The complete conversation history
//   - error: Any error encountered during execution
func (w *SimpleFlow) Run(ctx context.Context, client *Swarm) (string, []map[string]interface{}, error) {
	// Create workflow context with timeout
	wfCtx, cancel := context.WithTimeout(ctx, w.Timeout)
	defer cancel()

	// Initialize workflow
	if err := w.Initialize(); err != nil {
		return "", nil, fmt.Errorf("failed to initialize workflow: %w", err)
	}

	// Context variables to pass between steps
	contextVars := make(map[string]interface{})
	var messages []map[string]interface{}
	var lastContent string

	// Execute steps sequentially
	for i, step := range w.Steps {
		select {
		case <-wfCtx.Done():
			return "", nil, fmt.Errorf("workflow cancelled: %w", wfCtx.Err())
		default:
			// Execute single step
			result, err := w.executeStep(wfCtx, client, &step, contextVars, messages)
			if err != nil {
				if w.Verbose {
					fmt.Printf("Step %s failed: %v\n", step.Name, err)
				}
				return "", nil, fmt.Errorf("workflow failed at step %d (%s): %w", i+1, step.Name, err)
			}

			// Update state for next step
			if result != nil {
				messages = result.Messages
				lastContent = result.Content
				contextVars[fmt.Sprintf("%sResult", step.Name)] = result.Content
			}
		}
	}

	return lastContent, messages, nil
}
