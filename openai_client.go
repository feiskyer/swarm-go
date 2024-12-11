package swarm

import (
	"context"
	"fmt"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/azure"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/ssestream"
)

// OpenAIClient defines the interface for OpenAI API interactions
type OpenAIClient interface {
	CreateChatCompletion(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error)
	CreateChatCompletionStream(ctx context.Context, params openai.ChatCompletionNewParams) (*ssestream.Stream[openai.ChatCompletionChunk], error)
}

// openAIClientWrapper wraps the OpenAI client
type openAIClientWrapper struct {
	client *openai.Client
}

// NewOpenAIClient creates a new OpenAI client wrapper
func NewOpenAIClient(apiKey string) OpenAIClient {
	if apiKey == "" {
		return nil
	}

	return &openAIClientWrapper{
		client: openai.NewClient(option.WithAPIKey(apiKey)),
	}
}

// NewOpenAIClientWithBaseURL creates a new OpenAI client wrapper with a custom base URL
func NewOpenAIClientWithBaseURL(apiKey string, baseURL string) OpenAIClient {
	if apiKey == "" {
		return nil
	}

	if baseURL == "" {
		return &openAIClientWrapper{
			client: openai.NewClient(option.WithAPIKey(apiKey)),
		}
	}

	return &openAIClientWrapper{
		client: openai.NewClient(option.WithAPIKey(apiKey), option.WithBaseURL(baseURL)),
	}
}

// NewAzureOpenAIClient creates a new OpenAI client wrapper for Azure
func NewAzureOpenAIClient(apiKey, endpoint string) OpenAIClient {
	if apiKey == "" || endpoint == "" {
		return nil
	}

	return &openAIClientWrapper{
		client: openai.NewClient(
			azure.WithEndpoint(endpoint, "2024-06-01"),
			azure.WithAPIKey(apiKey),
		),
	}
}

// CreateChatCompletion implements OpenAIClient interface
func (c *openAIClientWrapper) CreateChatCompletion(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	completion, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to create chat completion: %w", err)
	}

	return completion, nil
}

// CreateChatCompletionStream implements OpenAIClient interface
func (c *openAIClientWrapper) CreateChatCompletionStream(ctx context.Context, params openai.ChatCompletionNewParams) (*ssestream.Stream[openai.ChatCompletionChunk], error) {
	if ctx == nil {
		ctx = context.Background()
	}

	stream := c.client.Chat.Completions.NewStreaming(ctx, params)
	if stream == nil {
		return nil, fmt.Errorf("failed to create streaming completion")
	}

	return stream, nil
}
