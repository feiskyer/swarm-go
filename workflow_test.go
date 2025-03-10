package swarm

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestWorkflow(t *testing.T) {
	workflow := NewWorkflow("test-workflow")
	workflow.WithConfig(WorkflowConfig{
		MaxTurns:   30,
		Timeout:    5 * time.Minute,
		MaxRetries: 3,
		Verbose:    true,
	})

	// Add test steps
	startStep := NewStep(
		"StartEventHandler",
		EventStart,
		func(ctx *Context, event Event) (Event, error) {
			if event.Type() != EventStart {
				return nil, fmt.Errorf("expected start event, got %s", event.Type())
			}

			tasks := []Task{
				{
					ID:      "task1",
					Type:    EventType("ProcessData"),
					Payload: map[string]interface{}{"data": "test1"},
					Timeout: time.Minute,
				},
				{
					ID:      "task2",
					Type:    EventType("ProcessData"),
					Payload: map[string]interface{}{"data": "test2"},
					Timeout: time.Minute,
				},
			}
			return NewParallelEvent(tasks, "ProcessData")
		},
		StepConfig{},
	)

	processStep := NewStep(
		"ProcessDataHandler",
		EventType("ProcessData"),
		func(ctx *Context, event Event) (Event, error) {
			data := event.Data()
			return NewBaseEvent(EventType("ProcessDataResult"), data), nil
		},
		StepConfig{
			MaxParallel: 2,
		},
	)

	// Add step to handle parallel results
	parallelResultStep := NewStep(
		"ParallelResultHandler",
		EventParallelResult,
		func(ctx *Context, event Event) (Event, error) {
			resultEvent := event.(*ParallelResultEvent)
			// Process parallel results here
			return NewBaseEvent(EventType("ProcessDataResult"), resultEvent.Results), nil
		},
		StepConfig{},
	)

	resultStep := NewStep(
		"ProcessDataResultHandler",
		EventType("ProcessDataResult"),
		func(ctx *Context, event Event) (Event, error) {
			return NewStopEvent(map[string]interface{}{"status": "success"}), nil
		},
		StepConfig{},
	)

	if err := workflow.AddStep(startStep); err != nil {
		t.Fatalf("Failed to add start step: %v", err)
	}
	if err := workflow.AddStep(processStep); err != nil {
		t.Fatalf("Failed to add process step: %v", err)
	}
	if err := workflow.AddStep(parallelResultStep); err != nil {
		t.Fatalf("Failed to add parallel result step: %v", err)
	}
	if err := workflow.AddStep(resultStep); err != nil {
		t.Fatalf("Failed to add result step: %v", err)
	}

	// Run workflow
	handler, err := workflow.Run(context.Background(), map[string]interface{}{
		"input": "test",
	})
	if err != nil {
		t.Fatalf("Failed to run workflow: %v", err)
	}

	// Wait for completion
	result, err := handler.Wait()
	if err != nil {
		t.Fatalf("Workflow execution failed: %v", err)
	}

	// Verify result
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	status, ok := resultMap["status"].(string)
	if !ok || status != "success" {
		t.Errorf("Expected status=success, got %v", status)
	}
}
