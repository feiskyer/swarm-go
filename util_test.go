package swarm

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestMapToStruct(t *testing.T) {
	type testStruct struct {
		Name    string `json:"name"`
		Age     int    `json:"age"`
		IsAdmin bool   `json:"is_admin"`
	}

	tests := []struct {
		name     string
		input    map[string]interface{}
		expected testStruct
		wantErr  bool
	}{
		{
			name: "valid conversion",
			input: map[string]interface{}{
				"name":     "John",
				"age":      30,
				"is_admin": true,
			},
			expected: testStruct{
				Name:    "John",
				Age:     30,
				IsAdmin: true,
			},
			wantErr: false,
		},
		{
			name: "invalid type",
			input: map[string]interface{}{
				"name":     "John",
				"age":      "invalid",
				"is_admin": true,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result testStruct
			err := mapToStruct(tt.input, &result)
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if result != tt.expected {
				t.Errorf("Expected %+v, got %+v", tt.expected, result)
			}
		})
	}
}

func TestFormatArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]interface{}
		expected string
	}{
		{
			name: "simple args",
			args: map[string]interface{}{
				"name": "John",
				"age":  30,
			},
			expected: "age=30, name=John",
		},
		{
			name:     "empty args",
			args:     map[string]interface{}{},
			expected: "",
		},
		{
			name: "complex args",
			args: map[string]interface{}{
				"nested": map[string]interface{}{
					"key": "value",
				},
				"array": []interface{}{1, 2, 3},
			},
			expected: "array=[1 2 3], nested=map[key:value]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatArgs(tt.args)
			// Sort both strings for comparison since map iteration order is random
			if sortString(result) != sortString(tt.expected) {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// Helper function to sort comma-separated strings
func sortString(s string) string {
	parts := strings.Split(s, ", ")
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

func TestStreamResponse(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]interface{}
		check func(*testing.T, StreamResponse)
	}{
		{
			name: "content only",
			input: map[string]interface{}{
				"content": "test content",
				"sender":  "TestAgent",
			},
			check: func(t *testing.T, sr StreamResponse) {
				if sr.Content != "test content" {
					t.Errorf("Expected content 'test content', got %q", sr.Content)
				}
				if sr.Sender != "TestAgent" {
					t.Errorf("Expected sender 'TestAgent', got %q", sr.Sender)
				}
			},
		},
		{
			name: "tool calls",
			input: map[string]interface{}{
				"tool_calls": []map[string]interface{}{
					{
						"function": map[string]interface{}{
							"name":      "test_func",
							"arguments": "{}",
						},
					},
				},
			},
			check: func(t *testing.T, sr StreamResponse) {
				if len(sr.ToolCalls) != 1 {
					t.Errorf("Expected 1 tool call, got %d", len(sr.ToolCalls))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sr StreamResponse
			if err := mapToStruct(tt.input, &sr); err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			tt.check(t, sr)
		})
	}
}

func TestFunctionToJSON(t *testing.T) {
	testFunc := func(args map[string]interface{}) (interface{}, error) {
		return "test", nil
	}

	result := FunctionToJSON(NewAgentFunction(
		"testFunc",
		"Test function description",
		testFunc,
		[]Parameter{{Name: "name", Type: reflect.TypeOf("string")}},
	))

	if result["type"] != "function" {
		t.Error("Expected type to be 'function'")
	}

	function, ok := result["function"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected function field to be a map")
	}

	if function["name"] == "" {
		t.Error("Expected non-empty function name")
	}

	params, ok := function["parameters"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected parameters field to be a map")
	}

	if params["type"] != "object" {
		t.Error("Expected parameters type to be 'object'")
	}
}

func TestMergeFields(t *testing.T) {
	target := map[string]interface{}{
		"a": "original",
		"nested": map[string]interface{}{
			"x": 1,
		},
	}

	source := map[string]interface{}{
		"a": "new",
		"b": "added",
		"nested": map[string]interface{}{
			"y": 2,
		},
	}

	MergeFields(target, source)

	if target["a"] != "new" {
		t.Error("Expected value to be overwritten")
	}

	if target["b"] != "added" {
		t.Error("Expected new field to be added")
	}

	nested, ok := target["nested"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected nested to remain a map")
	}

	if nested["x"] != 1 || nested["y"] != 2 {
		t.Error("Expected nested fields to be merged correctly")
	}
}

func TestGetJSONType(t *testing.T) {
	tests := []struct {
		input    reflect.Type
		expected string
	}{
		{input: reflect.TypeOf("string"), expected: "string"},
		{input: reflect.TypeOf(123), expected: "integer"},
		{input: reflect.TypeOf(int8(1)), expected: "integer"},
		{input: reflect.TypeOf(int16(1)), expected: "integer"},
		{input: reflect.TypeOf(int32(1)), expected: "integer"},
		{input: reflect.TypeOf(int64(1)), expected: "integer"},
		{input: reflect.TypeOf(uint(1)), expected: "integer"},
		{input: reflect.TypeOf(uint8(1)), expected: "integer"},
		{input: reflect.TypeOf(uint16(1)), expected: "integer"},
		{input: reflect.TypeOf(uint32(1)), expected: "integer"},
		{input: reflect.TypeOf(uint64(1)), expected: "integer"},
		{input: reflect.TypeOf(123.45), expected: "number"},
		{input: reflect.TypeOf(float32(1.23)), expected: "number"},
		{input: reflect.TypeOf(float64(1.23)), expected: "number"},
		{input: reflect.TypeOf(true), expected: "boolean"},
		{input: reflect.TypeOf([]int{}), expected: "array"},
		{input: reflect.TypeOf([3]int{}), expected: "array"},
		{input: reflect.TypeOf(map[string]interface{}{}), expected: "object"},
		{input: reflect.TypeOf(struct{}{}), expected: "object"},
		{input: reflect.TypeOf(interface{}(nil)), expected: "string"},
		{input: reflect.TypeOf(nil), expected: "string"},
	}

	for _, tt := range tests {
		got := getJSONType(tt.input)
		if got != tt.expected {
			t.Errorf("getJSONType(%v) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}
