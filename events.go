package swarm

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"time"
)

// EventType represents the type of an event in the workflow.
// It is used to identify and route different kinds of events through the workflow.
type EventType string

const (
	// EventStart indicates the beginning of a workflow execution
	EventStart EventType = "StartEvent"
	// EventStop signals the successful completion of a workflow
	EventStop EventType = "StopEvent"
	// EventError represents an error condition in the workflow
	EventError EventType = "ErrorEvent"
	// EventInputRequired signals that user input is needed to proceed
	EventInputRequired EventType = "InputRequiredEvent"
	// EventHumanResponse represents a response received from human interaction
	EventHumanResponse EventType = "HumanResponseEvent"
	// EventParallel indicates tasks that should be executed concurrently
	EventParallel EventType = "ParallelEvent"
	// EventParallelResult represents the aggregated results from parallel task execution
	EventParallelResult EventType = "ParallelResultEvent"
)

// EventValidator defines the interface for validating event data.
// Implementations should check if the event data meets required criteria.
type EventValidator interface {
	// Validate checks if the event data is valid.
	// Returns an error if validation fails, nil otherwise.
	Validate() error
}

// Event defines the core interface for all workflow events.
// Events are the primary mechanism for communication between workflow components.
type Event interface {
	// Type returns the event type name that identifies this event.
	Type() EventType

	// Data returns the event's associated data as a map.
	// The map contains event-specific information needed for processing.
	Data() map[string]interface{}

	// Validate checks if the event is properly configured.
	// Returns an error if validation fails, nil otherwise.
	Validate() error
}

// BaseEvent provides common functionality for all event types.
// It implements the basic Event interface and can be embedded in specific event types.
type BaseEvent struct {
	eventType EventType
	data      map[string]interface{}
}

// NewBaseEvent creates a new BaseEvent with the given event type and data.
func NewBaseEvent(eventType EventType, data map[string]interface{}) *BaseEvent {
	return &BaseEvent{
		eventType: eventType,
		data:      data,
	}
}

// NewEvent creates a new event of type T with the given event type and data.
func NewEvent[T any](eventType EventType, data T) *T {
	e := new(T)
	ev := reflect.ValueOf(e).Elem()

	// Find and initialize the BaseEvent field
	for i := 0; i < ev.NumField(); i++ {
		field := ev.Field(i)
		if field.Type() == reflect.TypeOf(&BaseEvent{}) {
			baseEvent := NewBaseEvent(eventType, nil)
			field.Set(reflect.ValueOf(baseEvent))
		}
	}

	// Copy data fields to the new event
	v := reflect.ValueOf(data)
	if v.Kind() == reflect.Struct {
		for i := 0; i < v.NumField(); i++ {
			fieldName := v.Type().Field(i).Name
			if fieldName != "BaseEvent" {
				if field := ev.FieldByName(fieldName); field.IsValid() && field.CanSet() {
					field.Set(v.Field(i))
				}
			}
		}
	}

	return e
}

// Type returns the event type
func (e *BaseEvent) Type() EventType {
	return e.eventType
}

// Data returns the event data
func (e *BaseEvent) Data() map[string]interface{} {
	if e.data == nil {
		e.data = make(map[string]interface{})
	}
	return e.data
}

// SetData replaces the entire event data map.
func (e *BaseEvent) SetData(data map[string]interface{}) {
	e.data = data
}

// Set stores a value in the event data with the given key.
func (e *BaseEvent) Set(key string, value interface{}) {
	if e.data == nil {
		e.data = make(map[string]interface{})
	}
	e.data[key] = value
}

// Get retrieves a value from the event data by key.
func (e *BaseEvent) Get(key string) interface{} {
	if e.data == nil {
		return nil
	}
	return e.data[key]
}

// Validate validates the base event
func (e *BaseEvent) Validate() error {
	if e.Type() == "" {
		return fmt.Errorf("event type is required")
	}
	return nil
}

// StartEvent represents the initialization of a workflow.
// It carries the initial inputs needed to begin workflow execution.
type StartEvent struct {
	BaseEvent
}

// NewStartEvent creates a new StartEvent with the given inputs.
func NewStartEvent(inputs map[string]interface{}) *StartEvent {
	return &StartEvent{
		BaseEvent: BaseEvent{
			eventType: EventStart,
			data:      inputs,
		},
	}
}

// Validate checks if the StartEvent is properly configured.
func (e *StartEvent) Validate() error {
	if err := e.BaseEvent.Validate(); err != nil {
		return err
	}
	if e.data == nil {
		return fmt.Errorf("inputs are required")
	}
	return nil
}

// StopEvent signals the successful completion of a workflow.
// It contains the final result of the workflow execution.
type StopEvent struct {
	BaseEvent
	// Result contains the final output of the workflow
	Result interface{} `json:"result"`
}

// NewStopEvent creates a new StopEvent with the given result.
func NewStopEvent(result interface{}) *StopEvent {
	return &StopEvent{
		BaseEvent: BaseEvent{
			eventType: EventStop,
		},
		Result: result,
	}
}

// Validate checks if the StopEvent is properly configured.
func (e *StopEvent) Validate() error {
	if err := e.BaseEvent.Validate(); err != nil {
		return err
	}
	if e.Result == nil {
		return fmt.Errorf("result is required")
	}
	return nil
}

// ErrorEvent represents an error in the workflow
type ErrorEvent struct {
	BaseEvent
	Error     error  `json:"error"`
	StepName  string `json:"step_name,omitempty"`
	TaskID    string `json:"task_id,omitempty"`
	Retriable bool   `json:"retriable"`
}

// NewErrorEvent creates a new ErrorEvent with the given error.
func NewErrorEvent(err error) *ErrorEvent {
	return &ErrorEvent{
		BaseEvent: BaseEvent{
			eventType: EventError,
		},
		Error:     err,
		Retriable: true, // Default to retriable
	}
}

// WithStep adds step information to the error event and returns the event.
func (e *ErrorEvent) WithStep(stepName string) *ErrorEvent {
	e.StepName = stepName
	return e
}

// WithTask adds task information to the error event and returns the event.
func (e *ErrorEvent) WithTask(taskID string) *ErrorEvent {
	e.TaskID = taskID
	return e
}

// WithRetriable sets whether the error is retriable and returns the event.
func (e *ErrorEvent) WithRetriable(retriable bool) *ErrorEvent {
	e.Retriable = retriable
	return e
}

// Validate checks if the ErrorEvent is properly configured.
func (e *ErrorEvent) Validate() error {
	if err := e.BaseEvent.Validate(); err != nil {
		return err
	}
	if e.Error == nil {
		return fmt.Errorf("error is required")
	}
	return nil
}

// TaskStatus represents the status of a task
type TaskStatus string

const (
	// TaskStatusPending indicates the task is waiting to be executed
	TaskStatusPending TaskStatus = "pending"
	// TaskStatusRunning indicates the task is currently being executed
	TaskStatusRunning TaskStatus = "running"
	// TaskStatusComplete indicates the task has completed successfully
	TaskStatusComplete TaskStatus = "complete"
	// TaskStatusFailed indicates the task has failed
	TaskStatusFailed TaskStatus = "failed"
	// TaskStatusCancelled indicates the task has been cancelled
	TaskStatusCancelled TaskStatus = "cancelled"
)

// Task represents a unit of work to be executed as part of a workflow.
// Tasks can be executed sequentially or in parallel depending on the workflow configuration.
type Task struct {
	// ID uniquely identifies the task
	ID string `json:"id"`
	// Type indicates the kind of task to be executed
	Type EventType `json:"type"`
	// Payload contains the data needed for task execution
	Payload interface{} `json:"payload"`
	// Status represents the current state of the task
	Status TaskStatus `json:"status"`
	// Error holds any error that occurred during task execution
	Error error `json:"error,omitempty"`
	// Priority determines the order of execution when multiple tasks are queued
	Priority int `json:"priority"`
	// Timeout specifies the maximum duration allowed for task execution
	Timeout time.Duration `json:"timeout"`
}

// NewTask creates a new task with default values
func NewTask(id string, eventType EventType, payload interface{}) Task {
	return Task{
		ID:       id,
		Type:     eventType,
		Payload:  payload,
		Status:   TaskStatusPending,
		Priority: 0,
		Timeout:  5 * time.Minute, // Default timeout
	}
}

// WithPriority sets the task priority and returns the task.
func (t Task) WithPriority(priority int) Task {
	t.Priority = priority
	return t
}

// WithTimeout sets the task timeout and returns the task.
func (t Task) WithTimeout(timeout time.Duration) Task {
	t.Timeout = timeout
	return t
}

// Validate validates the task configuration
func (t Task) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("task ID is required")
	}
	if t.Type == "" {
		return fmt.Errorf("task type is required")
	}
	return nil
}

// ParallelEvent represents an event that triggers parallel execution
type ParallelEvent struct {
	BaseEvent
	Tasks      []Task `json:"tasks"`
	SourceStep string `json:"source_step"` // Name of the step that generated this parallel event
}

// NewParallelEvent creates a new ParallelEvent with the given tasks and source step.
func NewParallelEvent(tasks []Task, sourceStep string) (*ParallelEvent, error) {
	// Validate tasks
	for _, task := range tasks {
		if err := task.Validate(); err != nil {
			return nil, fmt.Errorf("invalid task %s: %w", task.ID, err)
		}
	}

	// Sort tasks by priority (higher priority first)
	sortedTasks := make([]Task, len(tasks))
	copy(sortedTasks, tasks)
	sort.Slice(sortedTasks, func(i, j int) bool {
		return sortedTasks[i].Priority > sortedTasks[j].Priority
	})

	return &ParallelEvent{
		BaseEvent: BaseEvent{
			eventType: EventParallel,
		},
		Tasks:      sortedTasks,
		SourceStep: sourceStep,
	}, nil
}

// Validate checks if the ParallelEvent is properly configured.
func (e *ParallelEvent) Validate() error {
	if err := e.BaseEvent.Validate(); err != nil {
		return err
	}
	if len(e.Tasks) == 0 {
		return fmt.Errorf("at least one task is required")
	}
	for _, task := range e.Tasks {
		if err := task.Validate(); err != nil {
			return fmt.Errorf("invalid task %s: %w", task.ID, err)
		}
	}
	return nil
}

// GetTasks returns the tasks to be executed in parallel.
func (e *ParallelEvent) GetTasks() []Task {
	return e.Tasks
}

// ParallelResultEvent represents the results of parallel execution
type ParallelResultEvent struct {
	BaseEvent
	Results    map[string]interface{} `json:"results"`
	Errors     map[string]error       `json:"errors"`
	Successful int                    `json:"successful"`
	Failed     int                    `json:"failed"`
	Duration   time.Duration          `json:"duration"`
	SourceStep string                 `json:"source_step"` // Name of the step that generated the original parallel event
}

// NewParallelResultEvent creates a new ParallelResultEvent with the given results, errors, duration and source step.
func NewParallelResultEvent(results map[string]interface{}, errors map[string]error, duration time.Duration, sourceStep string) *ParallelResultEvent {
	successful := 0
	failed := 0
	for _, result := range results {
		if _, isError := result.(*ErrorEvent); isError {
			failed++
		} else {
			successful++
		}
	}

	return &ParallelResultEvent{
		BaseEvent: BaseEvent{
			eventType: EventParallelResult,
		},
		Results:    results,
		Errors:     errors,
		Successful: successful,
		Failed:     failed,
		Duration:   duration,
		SourceStep: sourceStep,
	}
}

// Validate checks if the ParallelResultEvent is properly configured.
func (e *ParallelResultEvent) Validate() error {
	if err := e.BaseEvent.Validate(); err != nil {
		return err
	}
	if e.Results == nil {
		return fmt.Errorf("results map is required")
	}
	if e.Errors == nil {
		return fmt.Errorf("errors map is required")
	}
	if e.Duration <= 0 {
		return fmt.Errorf("duration must be positive")
	}
	return nil
}

// GetResults returns the results of parallel execution.
func (e *ParallelResultEvent) GetResults() map[string]interface{} {
	return e.Results
}

// GetErrors returns the errors from parallel execution.
func (e *ParallelResultEvent) GetErrors() map[string]error {
	return e.Errors
}

// GetStats returns execution statistics including successful count, failed count and duration.
func (e *ParallelResultEvent) GetStats() (successful int, failed int, duration time.Duration) {
	return e.Successful, e.Failed, e.Duration
}

// ToMap converts an interface{} to map[string]interface{} using JSON marshaling.
func ToMap(v interface{}) (map[string]interface{}, error) {
	data := make(map[string]interface{})
	bytes, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal failed: %w", err)
	}
	if err := json.Unmarshal(bytes, &data); err != nil {
		return nil, fmt.Errorf("unmarshal failed: %w", err)
	}
	return data, nil
}

// ToStruct converts a map[string]interface{} to a struct using JSON marshaling.
func ToStruct(m map[string]interface{}, v interface{}) error {
	bytes, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal failed: %w", err)
	}
	if err := json.Unmarshal(bytes, v); err != nil {
		return fmt.Errorf("unmarshal failed: %w", err)
	}
	return nil
}
