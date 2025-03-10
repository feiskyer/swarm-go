package swarm

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"
)

// Workflow represents an executable workflow composed of ordered steps.
// Each workflow must have a start event handler as the entry point
// and at least one stop event to terminate execution.
//
// Required Event Types:
//   - EventStart: At least one step must handle start events
//   - EventStop: Workflow terminates when a stop event is received
//   - EventParallelResult: Required if using parallel execution
//
// Steps are mapped to events based on their EventType, allowing multiple
// steps to handle the same event type in parallel.
type Workflow struct {
	config  WorkflowConfig
	steps   []Step
	stepMap map[string][]Step
	mu      sync.RWMutex
}

// WorkflowConfig holds workflow-level configuration settings.
type WorkflowConfig struct {
	Name       string        `yaml:"name" json:"name"`
	MaxTurns   int           `yaml:"max_turns" json:"max_turns"`
	Verbose    bool          `yaml:"verbose" json:"verbose"`
	Timeout    time.Duration `yaml:"timeout" json:"timeout"`
	MaxRetries int           `yaml:"max_retries" json:"max_retries"`
}

// NewWorkflow creates a new workflow instance with the given name.
func NewWorkflow(name string) *Workflow {
	config := DefaultConfig()
	config.Name = name

	return &Workflow{
		config:  config,
		stepMap: make(map[string][]Step),
	}
}

// DefaultConfig returns default workflow configuration
func DefaultConfig() WorkflowConfig {
	return WorkflowConfig{
		MaxTurns:   30,
		Timeout:    5 * time.Minute,
		MaxRetries: 3,
	}
}

// WithConfig sets the workflow configuration and returns the workflow.
func (w *Workflow) WithConfig(config WorkflowConfig) *Workflow {
	w.config = config
	return w
}

// AddStep adds a step to the workflow. Returns an error if the step is invalid.
func (w *Workflow) AddStep(step Step) error {
	if err := w.validateStep(step); err != nil {
		return fmt.Errorf("invalid step: %w", err)
	}

	config := step.Config()
	if config.Timeout == 0 {
		config.Timeout = w.config.Timeout / time.Duration(len(w.steps)+1)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	w.steps = append(w.steps, step)
	w.stepMap[string(step.EventType())] = append(w.stepMap[string(step.EventType())], step)

	return nil
}

// validateStep validates a workflow step
func (w *Workflow) validateStep(step Step) error {
	if step.Name() == "" {
		return fmt.Errorf("step name is required")
	}

	config := step.Config()
	if config.MaxParallel < 0 {
		return fmt.Errorf("max parallel must be non-negative")
	}
	if config.Timeout < 0 {
		return fmt.Errorf("timeout must be non-negative")
	}
	if config.RetryPolicy != nil {
		if config.RetryPolicy.MaxRetries < 0 {
			return fmt.Errorf("max retries must be non-negative")
		}
		if config.RetryPolicy.InitialInterval <= 0 {
			return fmt.Errorf("initial interval must be positive")
		}
		if config.RetryPolicy.MaxInterval < config.RetryPolicy.InitialInterval {
			return fmt.Errorf("max interval must be greater than or equal to initial interval")
		}
		if config.RetryPolicy.Multiplier <= 0 {
			return fmt.Errorf("multiplier must be positive")
		}
	}
	return nil
}

// calculateBackoff calculates the next retry interval
func (p *RetryPolicy) calculateBackoff(attempt int) time.Duration {
	interval := p.InitialInterval * time.Duration(math.Pow(p.Multiplier, float64(attempt)))
	if interval > p.MaxInterval {
		interval = p.MaxInterval
	}
	return interval
}

// shouldRetry determines if an error should be retried
func (p *RetryPolicy) shouldRetry(err error) bool {
	if len(p.Errors) == 0 {
		return true // Retry all errors if no specific errors are specified
	}
	for _, retryErr := range p.Errors {
		if errors.Is(err, retryErr) {
			return true
		}
	}
	return false
}

// Initialize initializes the workflow
func (w *Workflow) Initialize() error {
	if w.config.MaxTurns == 0 {
		w.config.MaxTurns = 30
	}
	if w.config.Timeout == 0 {
		w.config.Timeout = 5 * time.Minute
	}
	if w.config.MaxRetries == 0 {
		w.config.MaxRetries = 3
	}
	return nil
}

// executeStep executes a single step with timeout and rate limiting
func (w *Workflow) executeStep(wfCtx *Context, step Step, event Event, sem *semaphore.Weighted) {
	config := step.Config()

	// Create step context with timeout
	stepCtx, cancel := context.WithTimeout(wfCtx.Context(), config.Timeout)
	defer cancel()

	// Acquire semaphore if rate limiting is enabled
	if sem != nil {
		if err := sem.Acquire(stepCtx, 1); err != nil {
			wfCtx.SendEvent(NewErrorEvent(fmt.Errorf("failed to acquire semaphore: %w", err)))
			return
		}
		defer sem.Release(1)
	}

	// Execute step with retries
	var result Event
	var lastErr error
	retryPolicy := config.RetryPolicy
	for i := 0; i < retryPolicy.MaxRetries; i++ {
		result, lastErr = step.Handle(wfCtx, event)
		if lastErr == nil {
			break
		}
		if w.config.Verbose {
			fmt.Printf("Step %s failed (attempt %d/%d): %v\n", step.Name(), i+1, retryPolicy.MaxRetries, lastErr)
		}
		if i < retryPolicy.MaxRetries-1 && retryPolicy.shouldRetry(lastErr) {
			backoff := retryPolicy.calculateBackoff(i)
			time.Sleep(backoff)
		} else {
			break
		}
	}

	if lastErr != nil {
		if w.config.Verbose {
			fmt.Printf("Step %s failed after %d retries: %v\n", step.Name(), retryPolicy.MaxRetries, lastErr)
		}
		wfCtx.SendEvent(NewErrorEvent(lastErr))
		return
	}

	if result != nil {
		wfCtx.SendEvent(result)
	}
}

// WorkflowStatus represents the current state of a workflow execution.
type WorkflowStatus string

const (
	// WorkflowStatusPending indicates the workflow is waiting to be executed
	WorkflowStatusPending WorkflowStatus = "pending"
	// WorkflowStatusRunning indicates the workflow is currently being executed
	WorkflowStatusRunning WorkflowStatus = "running"
	// WorkflowStatusComplete indicates the workflow has completed successfully
	WorkflowStatusComplete WorkflowStatus = "complete"
	// WorkflowStatusFailed indicates the workflow has failed
	WorkflowStatusFailed WorkflowStatus = "failed"
	// WorkflowStatusCancelled indicates the workflow has been cancelled
	WorkflowStatusCancelled WorkflowStatus = "cancelled"
)

// WorkflowHandler manages workflow execution and provides status updates.
type WorkflowHandler struct {
	ctx      *Context
	result   interface{}
	err      error
	doneChan chan struct{}
	errChan  chan error
	status   WorkflowStatus
	statusM  sync.RWMutex
}

// NewWorkflowHandler creates a new workflow handler
func NewWorkflowHandler(ctx *Context) *WorkflowHandler {
	return &WorkflowHandler{
		ctx:      ctx,
		doneChan: make(chan struct{}),
		errChan:  make(chan error, 1),
		status:   WorkflowStatusPending,
	}
}

// Wait blocks until the workflow completes and returns the result or error.
func (h *WorkflowHandler) Wait() (interface{}, error) {
	select {
	case <-h.doneChan:
		return h.result, h.err
	case err := <-h.errChan:
		return nil, err
	}
}

// Context returns the workflow context.
func (h *WorkflowHandler) Context() *Context {
	return h.ctx
}

// Stream returns a channel for receiving workflow events.
func (h *WorkflowHandler) Stream() <-chan Event {
	return h.ctx.Stream()
}

// Cancel stops workflow execution.
func (h *WorkflowHandler) Cancel() {
	h.ctx.Cancel()
}

// Status returns the current workflow status.
func (h *WorkflowHandler) Status() WorkflowStatus {
	h.statusM.RLock()
	defer h.statusM.RUnlock()
	return h.status
}

// setStatus sets the workflow status
func (h *WorkflowHandler) setStatus(status WorkflowStatus) {
	h.statusM.Lock()
	defer h.statusM.Unlock()
	h.status = status
}

// executeParallelTasks executes multiple tasks in parallel with rate limiting
func (w *Workflow) executeParallelTasks(wfCtx *Context, event *ParallelEvent, sem *semaphore.Weighted) {
	start := time.Now()
	results := make(map[string]interface{})
	errors := make(map[string]error)
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Create context with timeout
	ctx, cancel := context.WithTimeout(wfCtx.Context(), w.config.Timeout)
	defer cancel()

	// Process each task
	for _, task := range event.Tasks {
		wg.Add(1)
		go func(t Task) {
			defer wg.Done()

			taskCtx, taskCancel := context.WithTimeout(ctx, t.Timeout)
			defer taskCancel()

			// Update task status
			t.Status = TaskStatusRunning

			// Acquire semaphore if rate limiting is enabled
			if sem != nil {
				if err := sem.Acquire(taskCtx, 1); err != nil {
					t.Status = TaskStatusFailed
					t.Error = fmt.Errorf("failed to acquire semaphore: %w", err)
					mu.Lock()
					errors[t.ID] = t.Error
					results[t.ID] = NewErrorEvent(t.Error)
					mu.Unlock()
					return
				}
				defer sem.Release(1)
			}

			// Find matching steps for task type
			w.mu.RLock()
			steps := w.stepMap[string(t.Type)]
			w.mu.RUnlock()

			if len(steps) == 0 {
				t.Status = TaskStatusFailed
				t.Error = fmt.Errorf("no steps found for task type: %s", t.Type)
				mu.Lock()
				errors[t.ID] = t.Error
				results[t.ID] = NewErrorEvent(t.Error)
				mu.Unlock()
				return
			}

			// Create task event
			data, err := ToMap(t.Payload)
			if err != nil {
				t.Status = TaskStatusFailed
				t.Error = fmt.Errorf("failed to marshal task payload: %w", err)
				mu.Lock()
				errors[t.ID] = t.Error
				results[t.ID] = NewErrorEvent(t.Error)
				mu.Unlock()
				return
			}
			taskEvent := &BaseEvent{
				eventType: t.Type,
				data:      data,
			}

			// Execute each matching step with retries
			for _, step := range steps {
				var result Event
				var lastErr error
				retryPolicy := step.Config().RetryPolicy
				for i := 0; i < retryPolicy.MaxRetries; i++ {
					result, lastErr = step.Handle(wfCtx, taskEvent)
					if lastErr == nil {
						break
					}
					if w.config.Verbose {
						fmt.Printf("Task %s step %s failed (attempt %d/%d): %v\n", t.ID, step.Name(), i+1, retryPolicy.MaxRetries, lastErr)
					}
					if i < retryPolicy.MaxRetries-1 && retryPolicy.shouldRetry(lastErr) {
						backoff := retryPolicy.calculateBackoff(i)
						time.Sleep(backoff)
					} else {
						break
					}
				}

				if lastErr != nil {
					t.Status = TaskStatusFailed
					t.Error = lastErr
					if w.config.Verbose {
						fmt.Printf("Task %s step %s failed after %d retries: %v\n", t.ID, step.Name(), retryPolicy.MaxRetries, lastErr)
					}
					mu.Lock()
					errors[t.ID] = lastErr
					results[t.ID] = NewErrorEvent(lastErr)
					mu.Unlock()
					return
				}

				if result != nil {
					t.Status = TaskStatusComplete
					mu.Lock()
					results[t.ID] = result
					mu.Unlock()
				}
			}
		}(task)
	}

	// Wait for all tasks to complete
	wg.Wait()

	// Send parallel result event with execution stats
	duration := time.Since(start)
	wfCtx.SendEvent(NewParallelResultEvent(results, errors, duration, event.SourceStep))
}

// Run executes the workflow with the given context and input parameters.
// Returns a WorkflowHandler for monitoring execution.
func (w *Workflow) Run(ctx context.Context, inputs map[string]interface{}) (*WorkflowHandler, error) {
	if err := w.Initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize workflow: %w", err)
	}

	// Create workflow context with timeout
	wfCtx := NewContext(ctx)
	handler := NewWorkflowHandler(wfCtx)

	// Create WaitGroup to track step executions
	var wg sync.WaitGroup
	stepErrors := make(chan error, 1)

	// Start workflow in background
	go func() {
		defer func() {
			// Wait for all steps to complete
			done := make(chan struct{})
			go func() {
				wg.Wait()
				close(done)
			}()

			select {
			case <-done:
				// All steps completed successfully
				if handler.Status() != WorkflowStatusComplete && handler.Status() != WorkflowStatusFailed {
					handler.setStatus(WorkflowStatusComplete)
				}
			case err := <-stepErrors:
				// Step execution failed
				handler.err = err
				handler.errChan <- err
				handler.setStatus(WorkflowStatusFailed)
			}

			close(handler.doneChan)
			close(handler.errChan)
		}()

		// Update status
		handler.setStatus(WorkflowStatusRunning)

		// Send start event
		startEvent := NewStartEvent(inputs)
		wfCtx.SendEvent(startEvent)

		// Process events
		for {
			select {
			case <-ctx.Done():
				handler.err = ctx.Err()
				handler.errChan <- ctx.Err()
				handler.setStatus(WorkflowStatusCancelled)
				return

			case event := <-wfCtx.Events():
				if event == nil {
					handler.err = fmt.Errorf("received nil event")
					handler.errChan <- handler.err
					handler.setStatus(WorkflowStatusFailed)
					return
				}

				switch event.Type() {
				case EventStop:
					// Workflow complete
					stopEvent := event.(*StopEvent)
					handler.result = stopEvent.Result
					handler.setStatus(WorkflowStatusComplete)
					return

				case EventError:
					// Handle error event
					errorEvent := event.(*ErrorEvent)
					handler.err = errorEvent.Error
					handler.errChan <- errorEvent.Error
					handler.setStatus(WorkflowStatusFailed)
					return

				case EventParallel:
					// Handle parallel execution
					parallelEvent := event.(*ParallelEvent)
					maxParallel := int64(10) // Default to 10 parallel tasks
					sem := semaphore.NewWeighted(maxParallel)
					wg.Add(1)
					go func() {
						defer wg.Done()
						w.executeParallelTasks(wfCtx, parallelEvent, sem)
					}()

				case EventParallelResult:
					// Handle parallel result
					resultEvent := event.(*ParallelResultEvent)
					w.mu.RLock()
					steps := w.stepMap[string(EventParallelResult)]
					w.mu.RUnlock()

					if len(steps) == 0 {
						if w.config.Verbose {
							fmt.Printf("No steps found for parallel result handler\n")
						}
						continue
					}

					// Execute matching steps
					for _, step := range steps {
						wg.Add(1)
						go func(s Step) {
							defer wg.Done()
							w.executeStep(wfCtx, s, resultEvent, nil)
						}(step)
					}

				default:
					// Find matching steps
					w.mu.RLock()
					steps := w.stepMap[string(event.Type())]
					w.mu.RUnlock()

					if len(steps) == 0 {
						if w.config.Verbose {
							fmt.Printf("No steps found for event type: %s\n", event.Type())
						}
						continue
					}

					// Create rate limiter for parallel steps
					var sem *semaphore.Weighted
					maxParallel := int64(0)
					for _, step := range steps {
						if step.Config().MaxParallel > maxParallel {
							maxParallel = step.Config().MaxParallel
						}
					}
					if maxParallel > 0 {
						sem = semaphore.NewWeighted(maxParallel)
					}

					// Execute matching steps
					for _, step := range steps {
						wg.Add(1)
						go func(s Step) {
							defer wg.Done()
							w.executeStep(wfCtx, s, event, sem)
						}(step)
					}
				}
			}
		}
	}()

	return handler, nil
}

// NewStartStep creates a new start event handler step
func NewStartStep(handler StepFunc, retryPolicy *RetryPolicy) Step {
	config := StepConfig{
		RetryPolicy: retryPolicy,
	}
	if retryPolicy == nil {
		config.RetryPolicy = DefaultRetryPolicy()
	}

	return NewStep("StartEventHandler", EventStart, handler, config)
}
