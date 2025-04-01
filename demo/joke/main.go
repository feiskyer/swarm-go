package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/feiskyer/swarm-go"
)

// EventJoke defines a joke event
const EventJoke = swarm.EventType("JokeEvent")

// JokeEvent represents a generated joke
type JokeEvent struct {
	*swarm.BaseEvent
	Topic    string `json:"topic"`
	Joke     string `json:"joke"`
	Critique string `json:"critique,omitempty"`
}

// NewJokeEvent creates a new JokeEvent
func NewJokeEvent(topic, joke string, critique string) swarm.Event {
	return swarm.NewEvent(EventJoke, JokeEvent{
		Topic:    topic,
		Joke:     joke,
		Critique: critique,
	})
}

func handleStartEvent(ctx *swarm.Context, event swarm.Event, client *swarm.Swarm) (swarm.Event, error) {
	jokeGeneratorAgent := swarm.NewAgent("Joke Generator").WithInstructions(`
		You are a creative and witty joke generator. When given a topic, generate a clever and appropriate joke.
		Keep your responses family-friendly and engaging.
		Return only the joke without any additional commentary.
	`)

	if event.Type() != swarm.EventStart {
		return nil, fmt.Errorf("expected start event, got %s", event.Type())
	}

	data := event.Data()
	if data == nil {
		return nil, fmt.Errorf("no event data received")
	}

	topicVal, ok := data["topic"]
	if !ok {
		return nil, fmt.Errorf("topic not found in event data")
	}

	topic, ok := topicVal.(string)
	if !ok {
		return nil, fmt.Errorf("invalid topic type in event data")
	}

	ctx.Set("topic", topic)

	messages := []map[string]interface{}{
		{
			"role":    "user",
			"content": fmt.Sprintf("Generate a funny joke about %s.", topic),
		},
	}

	response, err := client.Run(ctx.Context(), jokeGeneratorAgent, messages, nil, "gpt-4", false, false, 10, true, false)
	if err != nil {
		return nil, fmt.Errorf("failed to generate joke: %w", err)
	}

	if len(response.Messages) == 0 {
		return nil, fmt.Errorf("no response messages received")
	}
	lastMsg := response.Messages[len(response.Messages)-1]
	content, ok := lastMsg["content"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid response content type")
	}

	joke := content
	ctx.Set("joke", joke)
	return NewJokeEvent(topic, joke, ""), nil
}

func handleJokeEvent(ctx *swarm.Context, event swarm.Event, client *swarm.Swarm) (swarm.Event, error) {
	jokeCriticAgent := swarm.NewAgent("Joke Critic").WithInstructions(`
		You are a thoughtful joke critic. When presented with a joke, analyze what makes it funny or not.
		Consider elements like wordplay, timing, relevance, and creativity.
		Provide a brief but insightful analysis.
	`)

	jokeEvent, ok := event.(*JokeEvent)
	if !ok {
		return nil, fmt.Errorf("expected JokeEvent, got %T", event)
	}

	messages := []map[string]interface{}{
		{
			"role":    "user",
			"content": fmt.Sprintf("Analyze this joke about %s:\n\n%s", jokeEvent.Topic, jokeEvent.Joke),
		},
	}

	response, err := client.Run(ctx.Context(), jokeCriticAgent, messages, nil, "gpt-4", false, false, 10, true, false)
	if err != nil {
		return nil, fmt.Errorf("failed to critique joke: %w", err)
	}

	if len(response.Messages) == 0 {
		return nil, fmt.Errorf("no response messages received")
	}

	lastMsg := response.Messages[len(response.Messages)-1]
	content, ok := lastMsg["content"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid response content type")
	}

	jokeEvent.Critique = content

	return swarm.NewStopEvent(jokeEvent), nil
}

func main() {
	client, err := swarm.NewDefaultSwarm()
	if err != nil {
		fmt.Printf("Failed to create client: %v\n", err)
		os.Exit(1)
	}

	// Create workflow
	workflow := swarm.NewWorkflow("joke-generator")
	workflow.WithConfig(swarm.WorkflowConfig{
		Name:       "joke-generator",
		Timeout:    5 * time.Minute,
		Verbose:    true,
		MaxTurns:   10,
		MaxRetries: 3,
	})

	// Add step to generate joke
	generateJokeStep := swarm.NewStep(
		"JokeGenerator",
		swarm.EventStart,
		func(ctx *swarm.Context, event swarm.Event) (swarm.Event, error) {
			return handleStartEvent(ctx, event, client)
		},
		swarm.StepConfig{
			RetryPolicy: &swarm.RetryPolicy{
				MaxRetries:      3,
				InitialInterval: time.Second,
				MaxInterval:     10 * time.Second,
				Multiplier:      2.0,
			},
		},
	)

	// Add step to critique joke
	critiqueJokeStep := swarm.NewStep(
		"JokeCritic",
		EventJoke,
		func(ctx *swarm.Context, event swarm.Event) (swarm.Event, error) {
			return handleJokeEvent(ctx, event, client)
		},
		swarm.StepConfig{
			RetryPolicy: &swarm.RetryPolicy{
				MaxRetries:      3,
				InitialInterval: time.Second,
				MaxInterval:     10 * time.Second,
				Multiplier:      2.0,
			},
		},
	)

	if err := workflow.AddStep(generateJokeStep); err != nil {
		fmt.Printf("Failed to add generate joke step: %v\n", err)
		os.Exit(1)
	}

	if err := workflow.AddStep(critiqueJokeStep); err != nil {
		fmt.Printf("Failed to add critique joke step: %v\n", err)
		os.Exit(1)
	}

	// Run workflow
	ctx := context.Background()
	startData := map[string]interface{}{
		"topic": "artificial intelligence",
	}
	handler, err := workflow.Run(ctx, startData)
	if err != nil {
		fmt.Printf("Failed to start workflow: %v\n", err)
		return
	}

	// Wait for result
	result, err := handler.Wait()
	if err != nil {
		fmt.Printf("Workflow failed: %v\n", err)
		return
	}

	if result != nil {
		data := result.(*JokeEvent)
		fmt.Printf("Topic: %s\n\n", data.Topic)
		fmt.Printf("Joke: %s\n\n", data.Joke)
		fmt.Printf("Critique: %s\n\n", data.Critique)
	}
}
