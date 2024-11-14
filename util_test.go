package swarm

import (
	"bytes"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
)

var stdout io.Writer = os.Stdout

func TestDebugPrint(t *testing.T) {
	// Redirect stdout to capture output
	var buf bytes.Buffer
	oldStdout := stdout
	stdout = &buf
	defer func() { stdout = oldStdout }()

	// Test with debug=false
	DebugPrint(false, "test message")
	if buf.String() != "" {
		t.Error("Expected no output when debug is false")
	}

	// Test with debug=true
	buf.Reset()
	DebugPrint(true, "test message")
	output := buf.String()

	if !strings.Contains(output, "test message") {
		t.Error("Expected output to contain test message")
	}
	if !strings.Contains(output, "\033[") {
		t.Error("Expected output to contain ANSI color codes")
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
	// Add test cases with non-nil values
	tests := []struct {
		input    interface{}
		expected string
	}{
		{input: "string", expected: "string"},
		{input: 123, expected: "number"},
		{input: true, expected: "boolean"},
		{input: map[string]interface{}{}, expected: "object"},
		{input: []interface{}{}, expected: "array"},
		{input: nil, expected: "null"},
	}

	for _, tt := range tests {
		got := getJSONType(reflect.TypeOf(tt.input))
		if got != tt.expected {
			t.Errorf("getJSONType(%v) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}
