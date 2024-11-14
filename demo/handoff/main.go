package main

import (
	"context"
	"fmt"
	"os"

	"github.com/feiskyer/swarm-go"
)

func createClient() (*swarm.Swarm, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey != "" {
		return swarm.NewSwarm(swarm.NewOpenAIClient(apiKey)), nil
	}

	azureApiKey := os.Getenv("AZURE_OPENAI_API_KEY")
	if azureApiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY or AZURE_OPENAI_API_KEY is not set")
	}

	azureApiBase := os.Getenv("AZURE_OPENAI_API_BASE")
	if azureApiBase == "" {
		return nil, fmt.Errorf("AZURE_OPENAI_API_BASE is not set")
	}

	return swarm.NewSwarm(swarm.NewAzureOpenAIClient(azureApiKey, azureApiBase)), nil
}

func main() {
	client, err := createClient()
	if err != nil {
		fmt.Println(err)
		return
	}

	englishAgent := swarm.NewAgent("English Agent").WithInstructions("You only speak English.")
	spanishAgent := swarm.NewAgent("Spanish Agent").WithInstructions("You only speak Spanish.")

	transferToSpanishAgent := swarm.NewAgentFunction(
		"transferToSpanishAgent",
		"Transfer spanish speaking users immediately.",
		func(args map[string]interface{}) (interface{}, error) {
			return spanishAgent, nil
		},
		[]swarm.Parameter{},
	)
	englishAgent.AddFunction(transferToSpanishAgent)

	messages := []map[string]interface{}{
		{
			"role":    "user",
			"content": "Hola. ¿Como estás?",
		},
	}
	response, err := client.Run(context.TODO(), englishAgent, messages, nil, "gpt-4o", false, true, 10, true)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(response.Messages[len(response.Messages)-1]["content"])
}
