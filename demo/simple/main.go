package main

import (
	"context"
	"fmt"
	"os"
	"reflect"

	"github.com/feiskyer/swarm-go"
)

func main() {
	// Create weather function
	weatherFunc := swarm.NewAgentFunction(
		"getWeather",
		"Get weather information for a location",
		func(args map[string]interface{}) (interface{}, error) {
			location, ok := args["location"].(string)
			if !ok {
				return nil, fmt.Errorf("location not provided")
			}
			return map[string]interface{}{
				"location":    location,
				"temperature": 72,
				"condition":   "sunny",
				"humidity":    45,
			}, nil
		},
		[]swarm.Parameter{
			{Name: "location", Type: reflect.TypeOf(""), Required: true},
		},
	)

	// Create a simple workflow
	workflow := &swarm.SimpleFlow{
		Name:     "weather-workflow",
		Model:    "gpt-4o",
		MaxTurns: 30,
		System:   "You are a weather assistant. Get weather information and recommendations, return in JSON format.",
		Steps: []swarm.SimpleFlowStep{
			{
				Name:         "get-weather",
				Instructions: "You are a weather assistant. Get weather information for the provided location and return it in JSON format.",
				Inputs: map[string]interface{}{
					"location": "Seattle",
				},
				Functions: []swarm.AgentFunction{weatherFunc},
			},
			{
				Name:         "analyze-weather",
				Instructions: "You are a weather analyst. Analyze the weather information and provide recommendations in JSON format.",
			},
		},
	}

	workflow.Initialize()

	// Create OpenAI client
	client, err := swarm.NewDefaultSwarm()
	if err != nil {
		fmt.Printf("Failed to create client: %v\n", err)
		os.Exit(1)
	}

	// Run workflow
	result, _, err := workflow.Run(context.Background(), client)
	if err != nil {
		fmt.Printf("Failed to run workflow: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(result)
}
