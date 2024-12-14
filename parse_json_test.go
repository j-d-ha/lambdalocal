//nolint:cyclop
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test cases struct
type testCase struct {
	input    map[string]any
	expected map[string]any
}

func TestParseInnerJSON(t *testing.T) {
	t.Parallel()

	tests := map[string]testCase{
		"nested JSON string": {
			input: map[string]any{
				"key1": `{"nestedKey1": "value1", "nestedKey2": 2}`,
				"key2": "plain string",
			},
			expected: map[string]any{
				"key1": map[string]any{
					"nestedKey1": "value1",
					"nestedKey2": 2.0,
				},
				"key2": "plain string",
			},
		},
		"invalid JSON string": {
			input: map[string]any{
				"key1": `{"nestedKey1": "value1", "nestedKey2": 2`,
				"key2": "plain string",
			},
			expected: map[string]any{
				"key1": `{"nestedKey1": "value1", "nestedKey2": 2`,
				"key2": "plain string",
			},
		},
		"non-string value": {
			input: map[string]any{
				"key1": 123,
				"key2": "plain string",
			},
			expected: map[string]any{
				"key1": 123,
				"key2": "plain string",
			},
		},
		"array inside a string": {
			input: map[string]any{
				"key1": `["value1", "value2", "value3"]`,
				"key2": "plain string",
			},
			expected: map[string]any{
				"key1": []any{"value1", "value2", "value3"},
				"key2": "plain string",
			},
		},
		"array as a value that's a string": {
			input: map[string]any{
				"key1": "[1, 2, 3]",
				"key2": `["a", "b", "c"]`,
			},
			expected: map[string]any{
				"key1": []any{1.0, 2.0, 3.0},
				"key2": []any{"a", "b", "c"},
			},
		},
	}

	for name, tc := range tests {
		t.Run(
			name, func(t *testing.T) {
				t.Parallel()

				actual := parseInnerJSON(tc.input)
				assert.Equal(t, tc.expected, actual)
			},
		)
	}
}
