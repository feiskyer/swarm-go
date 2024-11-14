package swarm

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/packages/ssestream"
)

// MockOpenAIClient mocks the OpenAI client for testing
type MockOpenAIClient struct {
	CompletionResponse *openai.ChatCompletion
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
	return m.CompletionResponse, nil
}

func (m *MockOpenAIClient) CreateChatCompletionStream(ctx context.Context, params openai.ChatCompletionNewParams) *ssestream.Stream[openai.ChatCompletionChunk] {
	if m.Error != nil {
		return nil
	}
	var buffer bytes.Buffer
	for _, chunk := range m.StreamResponse.chunks {
		buffer.WriteString(fmt.Sprintf("data: %+v\n\n", chunk))
	}
	httpRes := &http.Response{Body: io.NopCloser(&buffer)}
	return ssestream.NewStream[openai.ChatCompletionChunk](ssestream.NewDecoder(httpRes), nil)
}

// Next implements ChatCompletionStream interface
func (m *MockStream) Next() bool {
	m.current++
	return m.current < len(m.chunks)
}

// Current implements ChatCompletionStream interface
func (m *MockStream) Current() *openai.ChatCompletionChunk {
	if m.current >= len(m.chunks) {
		return nil
	}
	return m.chunks[m.current]
}

// Err implements ChatCompletionStream interface
func (m *MockStream) Err() error {
	return m.err
}

// Helper methods for testing
func (m *MockOpenAIClient) SetCompletionResponse(response *openai.ChatCompletion) {
	m.CompletionResponse = response
}

func (m *MockOpenAIClient) AddStreamChunk(chunk *openai.ChatCompletionChunk) {
	m.StreamResponse.chunks = append(m.StreamResponse.chunks, chunk)
}

func (m *MockOpenAIClient) SetError(err error) {
	m.Error = err
}

// Add these methods to fully implement ssestream.Stream
func (m *MockStream) IsClosed() bool {
	return m.closed
}
