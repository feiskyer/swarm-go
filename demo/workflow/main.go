package main

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/feiskyer/swarm-go"
)

func createClient() (*swarm.Swarm, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey != "" {
		return swarm.NewSwarm(swarm.NewOpenAIClient(apiKey)), nil
	}

	azureApiKey := os.Getenv("AZURE_OPENAI_API_KEY")
	azureApiBase := os.Getenv("AZURE_OPENAI_API_BASE")

	var missingEnvs []string
	if azureApiKey == "" {
		missingEnvs = append(missingEnvs, "AZURE_OPENAI_API_KEY")
	}
	if azureApiBase == "" {
		missingEnvs = append(missingEnvs, "AZURE_OPENAI_API_BASE")
	}

	if len(missingEnvs) > 0 {
		return nil, fmt.Errorf("required environment variables not set: %s", strings.Join(missingEnvs, ", "))
	}

	return swarm.NewSwarm(swarm.NewAzureOpenAIClient(azureApiKey, azureApiBase)), nil
}

func main() {
	// Create a workflow
	workflow := &swarm.Workflow{
		Name: "weather-workflow",
		Steps: []swarm.WorkflowStep{
			{
				Name:         "get-weather",
				Instructions: "You are a weather assistant. Get weather information for the provided location and return it in JSON format.",
				Model:        "gpt-4o",
				Inputs: map[string]interface{}{
					"location": "Seattle",
				},
			},
			{
				Name:         "analyze-weather",
				Instructions: "You are a weather analyst. Analyze the weather information and provide recommendations in JSON format.",
				Model:        "gpt-4o",
			},
		},
	}

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

	// Add function to the first step
	workflow.Steps[0].Functions = []swarm.AgentFunction{weatherFunc}
	workflow.Initialize()

	// Save workflow to YAML
	if err := workflow.SaveToYAML("weather-workflow.yaml"); err != nil {
		fmt.Printf("Failed to save workflow: %v\n", err)
		os.Exit(1)
	}

	// Create OpenAI client
	client, err := createClient()
	if err != nil {
		fmt.Printf("Failed to create client: %v\n", err)
		os.Exit(1)
	}

	// Run workflow
	result, err := workflow.Run(context.Background(), client)
	if err != nil {
		fmt.Printf("Failed to run workflow: %v\n", err)
		os.Exit(1)
	}

	// Print results
	fmt.Printf("\nWorkflow Results:\n")
	fmt.Printf("----------------\n")
	for _, stepResult := range result.Results {
		fmt.Printf("\nStep: %s\n", stepResult.StepName)
		if stepResult.Error != nil {
			fmt.Printf("Error: %v\n", stepResult.Error)
			continue
		}

		fmt.Printf("Outputs:\n")
		for k, v := range stepResult.Outputs {
			fmt.Printf("  %s: %v\n", k, v)
		}
	}
}
