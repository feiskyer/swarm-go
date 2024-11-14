package main

import (
	"fmt"
	"reflect"

	"github.com/feiskyer/swarm-go"
)

func main() {
	// Create a new agent
	agent := swarm.NewAgent("Assistant").WithModel("gpt-4o").
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
		[]swarm.Parameter{{Name: "location", Type: reflect.TypeOf("string"), Required: true}},
	)

	// Add function to agent
	agent.AddFunction(weatherFunc)

	// Run the demo loop
	swarm.RunDemoLoop(agent, nil, false, false)
}
