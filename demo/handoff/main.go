package main

import (
	"context"
	"fmt"
	"os"

	"github.com/feiskyer/swarm-go"
)

func main() {
	client, err := swarm.NewDefaultSwarm()
	if err != nil {
		fmt.Printf("Failed to create client: %v\n", err)
		os.Exit(1)
	}

	// Create agents with clear instructions
	englishAgent := swarm.NewAgent("English Agent").WithInstructions(`
		You are an English-speaking assistant. If a user speaks Spanish, immediately use the transferToSpanishAgent function.
		Do not attempt to translate or respond in Spanish yourself.
	`)

	spanishAgent := swarm.NewAgent("Spanish Agent").WithInstructions(`
		Eres un asistente que habla español. Responde a todas las preguntas en español.
		Si un usuario habla en inglés, continúa respondiendo en español pero adapta tu respuesta
		para ser lo más útil posible.
	`)

	// Create handoff function with clear description
	transferToSpanishAgent := swarm.NewAgentFunction(
		"transferToSpanishAgent",
		"Transfer the conversation to a Spanish-speaking agent when the user communicates in Spanish.",
		func(args map[string]interface{}) (interface{}, error) {
			return &swarm.Result{
				Value: "Transferring to Spanish-speaking agent...",
				Agent: spanishAgent,
			}, nil
		},
		[]swarm.Parameter{},
	)
	englishAgent.AddFunction(transferToSpanishAgent)

	// Initial message from user
	messages := []map[string]interface{}{
		{
			"role":    "user",
			"content": "Hola. ¿Como estás?",
		},
	}

	// Run the conversation with proper error handling
	ctx := context.Background()
	response, err := client.Run(ctx, englishAgent, messages, nil, "gpt-4o", false, true, 10, true)
	if err != nil {
		fmt.Printf("Error during conversation: %v\n", err)
		os.Exit(1)
	}

	// Print conversation history with proper formatting
	fmt.Println("\nConversation:")
	fmt.Println("-------------")
	for _, msg := range response.Messages {
		sender := msg["role"].(string)
		if msg["sender"] != nil {
			sender = msg["sender"].(string)
		}
		content := msg["content"].(string)
		fmt.Printf("%s: %s\n", sender, content)
	}
}
