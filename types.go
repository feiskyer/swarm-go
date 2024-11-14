package swarm

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/openai/openai-go"
)

// Common errors
var (
	ErrInvalidName        = fmt.Errorf("invalid name")
	ErrInvalidModel       = fmt.Errorf("invalid model")
	ErrInvalidFunction    = fmt.Errorf("invalid function")
	ErrInvalidParameter   = fmt.Errorf("invalid parameter")
	ErrInvalidInstruction = fmt.Errorf("invalid instruction type")
)

// AgentFunction represents a callable function that can be used by an agent.
type AgentFunction interface {
	// Call executes the function with given arguments
	Call(args map[string]interface{}) (interface{}, error)
	// Description returns the function's documentation
	Description() string
	// Name returns the function's name
	Name() string
	// Parameters returns the function's parameters
	Parameters() []Parameter
	// Validate checks if the function is properly configured
	Validate() error
}

// SimpleAgentFunction is a helper struct to create AgentFunction from a simple function
type SimpleAgentFunction struct {
	CallFn     func(map[string]interface{}) (interface{}, error)
	DescString string
	NameString string

	// TODO: auto infer parameters from function signature
	ParametersList []Parameter
}

func (f *SimpleAgentFunction) Call(args map[string]interface{}) (interface{}, error) {
	if f.CallFn == nil {
		return nil, fmt.Errorf("%w: CallFn is nil", ErrInvalidFunction)
	}
	return f.CallFn(args)
}

func (f *SimpleAgentFunction) Description() string {
	return f.DescString
}

func (f *SimpleAgentFunction) Name() string {
	return f.NameString
}

func (f *SimpleAgentFunction) Parameters() []Parameter {
	return f.ParametersList
}

func (f *SimpleAgentFunction) Validate() error {
	if f.CallFn == nil {
		return fmt.Errorf("%w: CallFn is nil", ErrInvalidFunction)
	}
	if f.NameString == "" {
		return fmt.Errorf("%w: name is empty", ErrInvalidFunction)
	}
	// if f.DescString == "" {
	// 	return fmt.Errorf("%w: description is empty", ErrInvalidFunction)
	// }
	// for _, p := range f.ParametersList {
	// 	if err := p.Validate(); err != nil {
	// 		return fmt.Errorf("parameter %q: %w", p.Name, err)
	// 	}
	// }
	return nil
}

// NewAgentFunction creates a new AgentFunction from a function and description
func NewAgentFunction(name string, desc string, fn func(map[string]interface{}) (interface{}, error), parameters []Parameter) AgentFunction {
	f := &SimpleAgentFunction{
		CallFn:         fn,
		DescString:     desc,
		NameString:     name,
		ParametersList: parameters,
	}
	return f
}

// Agent represents an AI agent with its configuration and capabilities.
type Agent struct {
	// Name identifies the agent
	Name string

	// Model specifies the OpenAI model to use (e.g., "gpt-4")
	Model string

	// Instructions can be either a string or a function returning a string
	// that provides the system message for the agent
	Instructions interface{}

	// Functions that this agent can call
	Functions []AgentFunction

	// ToolChoice specifies how the agent should use tools
	// Can be "none", "auto", or a specific function name
	ToolChoice *openai.ChatCompletionToolChoiceOptionUnionParam

	// ParallelToolCalls indicates if multiple tools can be called in parallel
	ParallelToolCalls bool

	// MaxTokens specifies the maximum number of tokens to generate
	MaxTokens int

	// Temperature controls randomness in responses (0.0 to 2.0)
	Temperature float32
}

// Response encapsulates the complete response from an agent interaction.
type Response struct {
	// Messages contains the conversation history
	Messages []map[string]interface{}

	// Agent is the current active agent (may change during conversation)
	Agent *Agent

	// ContextVariables stores shared context between function calls
	ContextVariables map[string]interface{}

	// TokensUsed tracks the number of tokens used in this response
	TokensUsed int

	// Cost tracks the estimated cost of this response
	Cost float64
}

// Result encapsulates the return value from an agent function.
type Result struct {
	// Value contains the function's string output
	Value string

	// Agent optionally specifies a new agent to switch to
	Agent *Agent

	// ContextVariables allows functions to update shared context
	ContextVariables map[string]interface{}

	// Error contains any error that occurred during function execution
	Error error
}

// NewAgent creates a new Agent with default values.
func NewAgent(name string) *Agent {
	if name == "" {
		return nil
	}

	return &Agent{
		Name:              name,
		Model:             "gpt-4",
		Instructions:      "You are a helpful agent.",
		Functions:         make([]AgentFunction, 0),
		ToolChoice:        nil,
		ParallelToolCalls: true,
		MaxTokens:         2000,
		Temperature:       0.7,
	}
}

// WithModel sets the model for the agent and returns the agent for chaining.
func (a *Agent) WithModel(model string) *Agent {
	if model == "" {
		return a
	}
	a.Model = model
	return a
}

// WithInstructions sets the instructions for the agent and returns the agent for chaining.
func (a *Agent) WithInstructions(instructions interface{}) *Agent {
	if instructions == nil {
		return a
	}
	a.Instructions = instructions
	return a
}

// WithMaxTokens sets the maximum tokens for the agent and returns the agent for chaining.
func (a *Agent) WithMaxTokens(tokens int) *Agent {
	if tokens <= 0 {
		return a
	}
	a.MaxTokens = tokens
	return a
}

// WithTemperature sets the temperature for the agent and returns the agent for chaining.
func (a *Agent) WithTemperature(temp float32) *Agent {
	if temp < 0 {
		return a
	}
	a.Temperature = temp
	return a
}

// AddFunction adds a function to the agent's capabilities and returns the agent for chaining.
func (a *Agent) AddFunction(f AgentFunction) *Agent {
	if f == nil {
		return a
	}
	if err := f.Validate(); err != nil {
		return a
	}
	a.Functions = append(a.Functions, f)
	return a
}

// Parameter represents a function parameter with its metadata
type Parameter struct {
	Name        string
	Description string
	Type        reflect.Type
	Required    bool
}

// Validate checks if the parameter is properly configured
func (p Parameter) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("%w: name is empty", ErrInvalidParameter)
	}
	if strings.TrimSpace(p.Description) == "" {
		return fmt.Errorf("%w: description is empty", ErrInvalidParameter)
	}
	if p.Type == nil {
		return fmt.Errorf("%w: type is nil", ErrInvalidParameter)
	}
	return nil
}
