# Swarm

An **ergonomic** and **lightweight** multi-agent orchestration framework inspired by [OpenAI's Swarm](https://github.com/openai/swarm). Designed for simplicity and efficiency, this framework empowers developers to build scalable and flexible multi-agent systems in Go.

**Features:**

- 🚀 **Lightweight Orchestration**: Efficiently manage multi-agent systems with a minimalistic and performant design. Perfect for applications where simplicity and speed are critical.
- 🛠️ **Native Function Calls**: Seamlessly integrate with your existing tools and services using native Go function calls. No complex wrappers or unnecessary abstractions—just straightforward integration.
- ⚡ **Event-Driven Workflows**: Build extensible and dynamic workflows driven by events. This approach ensures flexibility and adaptability for automating complex processes.
- 🧩 **Composable Architecture**: Create sophisticated systems by combining simple, reusable components. The framework’s modular design encourages clean and maintainable code.

## Getting Started

Add swarm to go mod by:

```shell
go get -u github.com/feiskyer/swarm-go
```

## Examples

Setup environment variables first:

- For OpenAI, set OPENAI_API_KEY and optional OPENAI_API_BASE for OpenAI API compatible AI service.
- For Azure OpenAI, set AZURE_OPENAI_API_KE and AZURE_OPENAI_API_BASE.

<details>
<summary>Basic Agent</summary>

A [basic agent](demo/basic/) with function calls:

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

</details>


<details>
<summary>Streaming Output</summary>

Use [streaming output](demo/streaming/) for your agent:

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

</details>


<details>
<summary>Multi-agent handoff</summary>

[Handoff example](demo/handoff/) for multiple agents:

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

</details>


<details>
<summary>Simple Workflow</summary>

Use [sequential simple workflow](demo/simple/).

</details>

<details>
<summary>Flexible Workflow</summary>

Use [flexible workflow](demo/joke/) for event-driven multi-agent orchestration.

</details>

<details>
<summary>Flexible workflow with parallel tasks</summary>

Use flexible workflow for event-driven multi-agent orchestration with [parallel tasks](demo/novel/).

</details>

## Contribution

The project is opensourced at github [feiskyer/swarm-go](https://github.com/feiskyer/swarm-go) with MIT License.

If you would like to contribute to the project, please follow these guidelines:

1. Fork the repository and clone it to your local machine.
2. Create a new branch for your changes.
3. Make your changes and commit them with a descriptive commit message.
4. Push your changes to your forked repository.
5. Open a pull request to the main repository.
