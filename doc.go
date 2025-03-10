// Package swarm provides functionality for orchestrating interactions between agents and OpenAI's language models.
// It implements a flexible framework for building AI-powered workflows and agent-based systems.
//
// The package supports:
//   - Agent-based interactions with OpenAI models
//   - Tool/function calling capabilities
//   - Streaming and non-streaming responses
//   - Context management and workflow orchestration
//   - Custom function execution
//   - Event-driven architecture for workflow management
//   - Parallel task execution and coordination
//   - Configurable retry policies and timeout handling
//
// Key Components:
//   - Agent: Represents an AI agent with specific instructions and capabilities
//   - Workflow: Manages the execution of sequential or parallel tasks
//   - Context: Handles state management and event propagation
//   - Events: Provides event types for workflow coordination
//   - OpenAI Client: Manages interactions with OpenAI's API
package swarm
