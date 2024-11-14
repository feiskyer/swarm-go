package swarm

import (
	"reflect"

	"github.com/openai/openai-go"
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
}

// SimpleAgentFunction is a helper struct to create AgentFunction from a simple function
type SimpleAgentFunction struct {
	CallFn         func(map[string]interface{}) (interface{}, error)
	DescString     string
	NameString     string
	ParametersList []Parameter
}

func (f *SimpleAgentFunction) Call(args map[string]interface{}) (interface{}, error) {
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

// NewAgentFunction creates a new AgentFunction from a function and description
func NewAgentFunction(name string, desc string, fn func(map[string]interface{}) (interface{}, error), parameters []Parameter) AgentFunction {
	return &SimpleAgentFunction{
		CallFn:         fn,
		DescString:     desc,
		NameString:     name,
		ParametersList: parameters,
	}
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
}

// Response encapsulates the complete response from an agent interaction.
type Response struct {
	// Messages contains the conversation history
	Messages []map[string]interface{}

	// Agent is the current active agent (may change during conversation)
	Agent *Agent

	// ContextVariables stores shared context between function calls
	ContextVariables map[string]interface{}
}

// Result encapsulates the return value from an agent function.
type Result struct {
	// Value contains the function's string output
	Value string

	// Agent optionally specifies a new agent to switch to
	Agent *Agent

	// ContextVariables allows functions to update shared context
	ContextVariables map[string]interface{}
}

// NewAgent creates a new Agent with default values.
func NewAgent(name string) *Agent {
	return &Agent{
		Name:              name,
		Model:             "gpt-4o",
		Instructions:      "You are a helpful agent.",
		Functions:         make([]AgentFunction, 0),
		ToolChoice:        nil,
		ParallelToolCalls: true,
	}
}

// WithModel sets the model for the agent and returns the agent for chaining.
func (a *Agent) WithModel(model string) *Agent {
	a.Model = model
	return a
}

// WithInstructions sets the instructions for the agent and returns the agent for chaining.
func (a *Agent) WithInstructions(instructions interface{}) *Agent {
	a.Instructions = instructions
	return a
}

// AddFunction adds a function to the agent's capabilities and returns the agent for chaining.
func (a *Agent) AddFunction(f AgentFunction) *Agent {
	a.Functions = append(a.Functions, f)
	return a
}

type Parameter struct {
	Name        string
	Description string
	Type        reflect.Type
}
