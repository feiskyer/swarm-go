# Swarm

An ergonomic, lightweight multi-agent orchestration in Go (inspired by [openai/swarm](https://github.com/openai/swarm)).

## Install

```shell
go get -u github.com/feiskyer/swarm-go
```

## Examples

### [Basic](demo/basic/main.go)

```go
package main

import (
	"fmt"
	"reflect"

	"github.com/feiskyer/swarm-go"
)

func main() {
	// Create a new agent
	agent := swarm.NewAgent("Assistant")
	agent.WithModel("gpt-4o").
		WithInstructions("You are a helpful assistant.")

	// Example function that the agent can call
	weatherFunc := swarm.NewAgentFunction(
		"getWeather",
		"Get the current weather for a given location. Requires a location parameter.",
		func(args map[string]interface{}) (interface{}, error) {
			location, ok := args["location"].(string)
			if !ok {
				return nil, fmt.Errorf("location not provided")
			}
			return fmt.Sprintf("The weather in %s is sunny", location), nil
		},
		[]swarm.Parameter{{Name: "location", Type: reflect.TypeOf("string")}},
	)

	// Add function to agent
	agent.AddFunction(weatherFunc)

	// Run the demo loop
	swarm.RunDemoLoop(agent, nil, false, false)
}
```

## [Streaming](demo/streaming/main.go)

```go
package main

import (
	"fmt"
	"reflect"

	"github.com/feiskyer/swarm-go"
)

func main() {
	// Create a new agent
	agent := swarm.NewAgent("Assistant")
	agent.WithModel("gpt-4o").
		WithInstructions("You are a helpful assistant.")

	// Example function that the agent can call
	weatherFunc := func(args map[string]interface{}) (interface{}, error) {
		location, ok := args["location"].(string)
		if !ok {
			return nil, fmt.Errorf("location not provided")
		}
		return fmt.Sprintf("The weather in %s is sunny", location), nil
	}

	// Add function to agent
	agent.AddFunction(swarm.NewAgentFunction(
		"getWeather",
		"Get the current weather for a given location. Requires a location parameter.",
		weatherFunc,
		[]swarm.Parameter{{Name: "location", Type: reflect.TypeOf("string")}},
	))

	// Run the demo loop
	swarm.RunDemoLoop(agent, nil, true, false)
}
```

## [Agent Handoff](demo/handoff/main.go)

```go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/feiskyer/swarm-go"
)

func main() {
	client := swarm.NewSwarm(swarm.NewOpenAIClient(os.Getenv("OPENAI_API_KEY")))

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
```

## [Use Azure OpenAI](demo/handoff/main.go)

```go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/feiskyer/swarm-go"
)

func main() {
	azureApiKey := os.Getenv("AZURE_OPENAI_API_KEY")
	azureApiBase := os.Getenv("AZURE_OPENAI_API_BASE")
	client := swarm.NewSwarm(swarm.NewAzureOpenAIClient(azureApiKey, azureApiBase))

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
```

## Contribution

The project is opensourced at github [feiskyer/swarm-go](https://github.com/feiskyer/swarm-go) with MIT License.

If you would like to contribute to the project, please follow these guidelines:

1. Fork the repository and clone it to your local machine.
2. Create a new branch for your changes.
3. Make your changes and commit them with a descriptive commit message.
4. Push your changes to your forked repository.
5. Open a pull request to the main repository.
