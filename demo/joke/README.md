# Joke Workflow Example

This example demonstrates how to use the event-based workflow system to create a simple joke generation and critique workflow.

## Overview

The workflow consists of two steps:

1. `generate_joke`: Takes a topic and generates a joke about it using GPT-4
2. `critique_joke`: Takes the generated joke and provides a thorough analysis and critique

## Events

The workflow uses the following events:

- `StartEvent`: Initial event with the topic to generate a joke about
- `JokeEvent`: Contains the generated joke
- `StopEvent`: Final event with the joke critique

## Running the Example

1. Set your OpenAI API key in the code (replace `YOUR_API_KEY`)

2. Run the example:

```bash
go run main.go
```

The workflow will:
1. Generate a joke about pirates
2. Print the generated joke
3. Analyze and critique the joke
4. Print the critique

## Example Output

```
Generated joke: Why don't pirates take a shower before they walk the plank? 
Because they'll wash up on shore later anyway!

Critique: This joke demonstrates classic elements of pirate-themed humor with a clever play on words:

1. Structure: The joke follows a traditional setup/punchline format
2. Wordplay: Uses "wash up" in both its literal (cleaning) and figurative (bodies washing ashore) meanings
3. Dark humor: Makes light of the serious topic of walking the plank
4. Thematic consistency: Incorporates multiple pirate elements (plank walking, shores)
5. Timing: The punchline is concise and delivers the twist effectively

Overall, it's a solid joke that balances dark humor with playful wordplay, though some might find the reference to death a bit macabre. 