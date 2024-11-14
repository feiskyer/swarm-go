package swarm

import (
	"context"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/azure"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/ssestream"
)

// OpenAIClient defines the interface for OpenAI API interactions
type OpenAIClient interface {
	CreateChatCompletion(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error)
	CreateChatCompletionStream(ctx context.Context, params openai.ChatCompletionNewParams) *ssestream.Stream[openai.ChatCompletionChunk]
}

// openAIClientWrapper wraps the OpenAI client
type openAIClientWrapper struct {
	client *openai.Client
}

// NewOpenAIClient creates a new OpenAI client wrapper
func NewOpenAIClient(apiKey string) OpenAIClient {
	return &openAIClientWrapper{
		client: openai.NewClient(option.WithAPIKey(apiKey)),
	}
}

// NewAzureOpenAIClient creates a new OpenAI client wrapper for Azure
func NewAzureOpenAIClient(apiKey, endpoint string) OpenAIClient {
	return &openAIClientWrapper{
		client: openai.NewClient(
			azure.WithEndpoint(endpoint, "2024-06-01"),
			azure.WithAPIKey(apiKey),
		),
	}
}

// CreateChatCompletion implements OpenAIClient interface
func (c *openAIClientWrapper) CreateChatCompletion(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	return c.client.Chat.Completions.New(ctx, params)
}

// CreateChatCompletionStream implements OpenAIClient interface
func (c *openAIClientWrapper) CreateChatCompletionStream(ctx context.Context, params openai.ChatCompletionNewParams) *ssestream.Stream[openai.ChatCompletionChunk] {
	return c.client.Chat.Completions.NewStreaming(ctx, params)
}
