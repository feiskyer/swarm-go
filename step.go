package swarm

import (
	"time"
)

// Step defines the interface for workflow steps
type Step interface {
	// Name returns the step's unique identifier
	Name() string

	// EventType returns the type of event this step handles
	EventType() EventType

	// Config returns the step's configuration
	Config() StepConfig

	// Handle processes an event and returns a new event or error
	Handle(ctx *Context, event Event) (Event, error)
}

// RetryPolicy configures step execution retry behavior using exponential backoff.
type RetryPolicy struct {
	// MaxRetries is the maximum number of retry attempts
	MaxRetries int

	// InitialInterval is the delay before first retry
	InitialInterval time.Duration

	// MaxInterval caps the maximum delay between retries
	MaxInterval time.Duration

	// Multiplier controls exponential backoff rate
	Multiplier float64

	// Errors specifies which errors trigger retries. Empty means all errors.
	Errors []error
}

// StepConfig holds step configuration settings
type StepConfig struct {
	MaxParallel int64
	Timeout     time.Duration
	RetryPolicy *RetryPolicy
}

// StepFunc represents a workflow step function that processes an event and returns a new event or error.
// The function receives a workflow context and an input event.
type StepFunc func(ctx *Context, event Event) (Event, error)

// BaseStep provides common step functionality
type BaseStep struct {
	name      string
	handler   StepFunc
	config    StepConfig
	eventType EventType
}

// Name returns the step's name
func (s *BaseStep) Name() string {
	return s.name
}

// Handle executes the step's handler function
func (s *BaseStep) Handle(ctx *Context, event Event) (Event, error) {
	return s.handler(ctx, event)
}

// Config returns the step's configuration
func (s *BaseStep) Config() StepConfig {
	return s.config
}

// EventType returns the type of event this step handles
func (s *BaseStep) EventType() EventType {
	return s.eventType
}

// NewStep creates a new step with the given configuration
func NewStep(name string, eventType EventType, handler StepFunc, config StepConfig) Step {
	if config.RetryPolicy == nil {
		config.RetryPolicy = DefaultRetryPolicy()
	}
	return &BaseStep{
		name:      name,
		handler:   handler,
		config:    config,
		eventType: eventType,
	}
}

// DefaultRetryPolicy returns the default retry policy
func DefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxRetries:      3,
		InitialInterval: time.Second,
		MaxInterval:     30 * time.Second,
		Multiplier:      2.0,
	}
}
