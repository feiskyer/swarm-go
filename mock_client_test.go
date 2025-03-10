package swarm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/packages/ssestream"
)

// MockOpenAIClient mocks the OpenAI client for testing
type MockOpenAIClient struct {
	CompletionIter     int
	CompletionResponse []*openai.ChatCompletion
	StreamResponse     *MockStream
	Error              error
}

// MockStream implements openai.ChatCompletionStream interface
type MockStream struct {
	chunks  []*openai.ChatCompletionChunk
	current int
	err     error
	closed  bool
}

// Close implements Stream interface
func (m *MockStream) Close() error {
	m.closed = true
	return nil
}

func NewMockOpenAIClient() *MockOpenAIClient {
	return &MockOpenAIClient{
		StreamResponse: &MockStream{
			chunks: make([]*openai.ChatCompletionChunk, 0),
		},
	}
}

func (m *MockOpenAIClient) CreateChatCompletion(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	if m.Error != nil {
		return nil, m.Error
	}
	iter := m.CompletionIter
	m.CompletionIter++
	return m.CompletionResponse[iter], nil
}

func (m *MockOpenAIClient) CreateChatCompletionStream(ctx context.Context, params openai.ChatCompletionNewParams) (*ssestream.Stream[openai.ChatCompletionChunk], error) {
	if m.Error != nil {
		return nil, m.Error
	}

	// Return a new stream that wraps our mock stream
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		for _, chunk := range m.StreamResponse.chunks {
			chunkData := map[string]interface{}{
				"id":      "mock",
				"object":  "chat.completion.chunk",
				"created": 0,
				"model":   "mock",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"delta": map[string]interface{}{
							"content": chunk.Choices[0].Delta.Content,
							"role":    "assistant",
						},
						"finish_reason": nil,
					},
				},
			}

			// If the chunk has a function call, convert it to a tool call
			if fc := chunk.Choices[0].Delta.FunctionCall; fc.Name != "" || fc.Arguments != "" {
				chunkData["choices"].([]map[string]interface{})[0]["delta"] = map[string]interface{}{
					"tool_calls": []map[string]interface{}{
						{
							"index": 0,
							"id":    "call_1",
							"type":  "function",
							"function": map[string]interface{}{
								"name":      fc.Name,
								"arguments": fc.Arguments,
							},
						},
					},
				}
			}

			chunkJSON, _ := json.Marshal(chunkData)
			pw.Write([]byte("data: "))
			pw.Write(chunkJSON)
			pw.Write([]byte("\n\n"))
		}
		// Send the final chunk with finish_reason
		finalChunk, _ := json.Marshal(map[string]interface{}{
			"id":      "mock",
			"object":  "chat.completion.chunk",
			"created": 0,
			"model":   "mock",
			"choices": []map[string]interface{}{
				{
					"index":         0,
					"delta":         map[string]interface{}{},
					"finish_reason": "stop",
				},
			},
		})
		pw.Write([]byte("data: "))
		pw.Write(finalChunk)
		pw.Write([]byte("\n\n"))
	}()

	httpRes := &http.Response{Body: pr}
	return ssestream.NewStream[openai.ChatCompletionChunk](ssestream.NewDecoder(httpRes), nil), nil
}

func (m *MockOpenAIClient) SetCompletionResponse(response *openai.ChatCompletion) {
	m.CompletionResponse = append(m.CompletionResponse, response)
}

func (m *MockOpenAIClient) AddStreamChunk(chunk *openai.ChatCompletionChunk) {
	m.StreamResponse.chunks = append(m.StreamResponse.chunks, chunk)
}

func (m *MockOpenAIClient) SetError(err error) {
	m.Error = err
}

func (m *MockStream) IsClosed() bool {
	return m.closed
}

// Next implements ChatCompletionStream interface
func (m *MockStream) Next() bool {
	return m.current < len(m.chunks)
}

// Current implements ChatCompletionStream interface
func (m *MockStream) Current() *openai.ChatCompletionChunk {
	if m.current >= len(m.chunks) {
		return nil
	}
	chunk := m.chunks[m.current]
	m.current++
	return chunk
}

// Err implements ChatCompletionStream interface
func (m *MockStream) Err() error {
	return m.err
}
