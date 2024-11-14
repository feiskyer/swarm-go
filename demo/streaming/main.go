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
