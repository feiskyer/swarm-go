package swarm

import (
	"context"
	"fmt"
	"sync"
)

// Context represents a workflow execution context that manages state and event flow.
// It wraps a standard context.Context and provides additional functionality for
// event handling, state management, and workflow control.
//
// The Context is safe for concurrent use by multiple goroutines.
type Context struct {
	ctx       context.Context
	cancel    context.CancelFunc
	eventChan chan Event
	streamCh  chan Event
	state     map[string]interface{}
	mu        sync.RWMutex
}

// NewContext creates a new workflow Context with the provided parent context.
// It initializes event channels with a buffer size of 100 and creates an empty state map.
func NewContext(ctx context.Context) *Context {
	ctx, cancel := context.WithCancel(ctx)
	return &Context{
		ctx:       ctx,
		cancel:    cancel,
		eventChan: make(chan Event, 100), // Buffer size of 100
		streamCh:  make(chan Event, 100), // Buffer size of 100
		state:     make(map[string]interface{}),
	}
}

// Context returns the underlying context.Context that this Context wraps.
func (c *Context) Context() context.Context {
	return c.ctx
}

// Cancel cancels the Context and all operations using it.
// After calling Cancel, all event channels will be closed and subsequent operations
// will return context.Canceled error.
func (c *Context) Cancel() {
	c.cancel()
}

// SendEvent sends an event to the workflow's event channel.
// It validates the event before sending and returns an error if the event is nil
// or invalid. The event is also sent to the stream channel if there are listeners.
//
// Returns an error if the context is canceled or if the event is invalid.
func (c *Context) SendEvent(event Event) error {
	if event == nil {
		return fmt.Errorf("event cannot be nil")
	}

	// Validate event
	if err := event.Validate(); err != nil {
		return fmt.Errorf("invalid event: %w", err)
	}

	select {
	case <-c.ctx.Done():
		return c.ctx.Err()
	case c.eventChan <- event:
		// Also send to stream channel if anyone is listening
		select {
		case c.streamCh <- event:
		default:
			// Stream channel is full or no one is listening, skip
		}
		return nil
	}
}

// Events returns a receive-only channel for consuming workflow events.
// The channel has a buffer size of 100 events.
func (c *Context) Events() <-chan Event {
	return c.eventChan
}

// Stream returns a receive-only channel for streaming workflow events.
// Unlike Events(), this channel is intended for real-time monitoring and may drop
// events if no receiver is ready.
func (c *Context) Stream() <-chan Event {
	return c.streamCh
}

// Set stores a key-value pair in the Context's state map.
// The operation is thread-safe and will overwrite any existing value for the key.
func (c *Context) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state[key] = value
}

// Get retrieves a value from the Context's state map.
// Returns the value and a boolean indicating whether the key was found.
func (c *Context) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	value, ok := c.state[key]
	return value, ok
}

// GetString retrieves a string value from the Context's state map.
// Returns the string value and a boolean indicating whether the key was found
// and the value was of type string.
func (c *Context) GetString(key string) (string, bool) {
	value, ok := c.Get(key)
	if !ok {
		return "", false
	}
	str, ok := value.(string)
	return str, ok
}

// GetInt retrieves an int value from the Context's state map.
// Returns the int value and a boolean indicating whether the key was found
// and the value was of type int.
func (c *Context) GetInt(key string) (int, bool) {
	value, ok := c.Get(key)
	if !ok {
		return 0, false
	}
	i, ok := value.(int)
	return i, ok
}

// GetBool retrieves a bool value from the Context's state map.
// Returns the bool value and a boolean indicating whether the key was found
// and the value was of type bool.
func (c *Context) GetBool(key string) (bool, bool) {
	value, ok := c.Get(key)
	if !ok {
		return false, false
	}
	b, ok := value.(bool)
	return b, ok
}

// GetMap retrieves a map value from the Context's state map.
// Returns the map value and a boolean indicating whether the key was found
// and the value was of type map[string]interface{}.
func (c *Context) GetMap(key string) (map[string]interface{}, bool) {
	value, ok := c.Get(key)
	if !ok {
		return nil, false
	}
	m, ok := value.(map[string]interface{})
	return m, ok
}

// Delete removes a key and its associated value from the Context's state map.
// If the key doesn't exist, the operation is a no-op.
func (c *Context) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.state, key)
}

// Clear removes all key-value pairs from the Context's state map.
// This operation is atomic and thread-safe.
func (c *Context) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state = make(map[string]interface{})
}

// Keys returns a slice containing all keys present in the Context's state map.
// The order of keys in the returned slice is not guaranteed to be stable.
func (c *Context) Keys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	keys := make([]string, 0, len(c.state))
	for k := range c.state {
		keys = append(keys, k)
	}
	return keys
}

// Len returns the number of key-value pairs in the Context's state map.
func (c *Context) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.state)
}

// Has checks if a key exists in the Context's state map.
// Returns true if the key exists, false otherwise.
func (c *Context) Has(key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.state[key]
	return ok
}

// Clone creates and returns a deep copy of the Context's state map.
// The returned map is independent of the Context and can be safely modified.
func (c *Context) Clone() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	clone := make(map[string]interface{}, len(c.state))
	for k, v := range c.state {
		clone[k] = v
	}
	return clone
}
