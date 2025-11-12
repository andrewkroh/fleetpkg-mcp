// Licensed to Elasticsearch B.V. under one or more agreements.
// Elasticsearch B.V. licenses this file to you under the Apache 2.0 License.
// See the LICENSE file in the project root for more information.

package fleetsql

import (
	"encoding/json"
	"testing"

	"github.com/andrewkroh/go-fleetpkg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlattenProcessors(t *testing.T) {
	tests := []struct {
		name      string
		input     []*fleetpkg.Processor
		basePath  string
		wantCount int
		validate  func(t *testing.T, result []FlatProcessor)
	}{
		{
			name:      "empty processors",
			input:     []*fleetpkg.Processor{},
			basePath:  "/processors",
			wantCount: 0,
		},
		{
			name: "single processor without on_failure",
			input: []*fleetpkg.Processor{
				{
					Type: "set",
					Attributes: map[string]any{
						"field": "test.field",
						"value": "test_value",
					},
				},
			},
			basePath:  "/processors",
			wantCount: 1,
			validate: func(t *testing.T, result []FlatProcessor) {
				assert.Equal(t, "set", result[0].Type)
				assert.Equal(t, "/processors/0", result[0].JSONPointer)
				assert.Equal(t, "test.field", result[0].Attributes["field"])
				assert.NotContains(t, result[0].Attributes, "on_failure")
			},
		},
		{
			name: "processor with on_failure handlers",
			input: []*fleetpkg.Processor{
				{
					Type: "rename",
					Attributes: map[string]any{
						"field":        "old_field",
						"target_field": "new_field",
					},
					OnFailure: []*fleetpkg.Processor{
						{
							Type: "set",
							Attributes: map[string]any{
								"field": "error.message",
								"value": "Rename failed",
							},
						},
					},
				},
			},
			basePath:  "/processors",
			wantCount: 2, // Parent + on_failure handler
			validate: func(t *testing.T, result []FlatProcessor) {
				// First should be the on_failure handler
				assert.Equal(t, "set", result[0].Type)
				assert.Equal(t, "/processors/0/on_failure/0", result[0].JSONPointer)
				assert.Equal(t, "error.message", result[0].Attributes["field"])

				// Second should be the parent processor
				assert.Equal(t, "rename", result[1].Type)
				assert.Equal(t, "/processors/0", result[1].JSONPointer)
				assert.Contains(t, result[1].Attributes, "on_failure")

				// Verify on_failure is properly serialized
				onFailure, ok := result[1].Attributes["on_failure"].([]map[string]any)
				require.True(t, ok)
				require.Len(t, onFailure, 1)
				assert.Contains(t, onFailure[0], "set")
			},
		},
		{
			name: "nested on_failure handlers",
			input: []*fleetpkg.Processor{
				{
					Type: "grok",
					Attributes: map[string]any{
						"field": "message",
					},
					OnFailure: []*fleetpkg.Processor{
						{
							Type: "set",
							Attributes: map[string]any{
								"field": "error.type",
								"value": "parse_error",
							},
							OnFailure: []*fleetpkg.Processor{
								{
									Type: "append",
									Attributes: map[string]any{
										"field": "tags",
										"value": "parse_failure",
									},
								},
							},
						},
					},
				},
			},
			basePath:  "/processors",
			wantCount: 3, // Parent + on_failure + nested on_failure
			validate: func(t *testing.T, result []FlatProcessor) {
				// Check JSON Pointers are correct
				pointers := make([]string, len(result))
				for i, p := range result {
					pointers[i] = p.JSONPointer
				}
				assert.Contains(t, pointers, "/processors/0/on_failure/0/on_failure/0")
				assert.Contains(t, pointers, "/processors/0/on_failure/0")
				assert.Contains(t, pointers, "/processors/0")
			},
		},
		{
			name: "multiple processors with mixed on_failure",
			input: []*fleetpkg.Processor{
				{
					Type: "set",
					Attributes: map[string]any{
						"field": "status",
						"value": "ok",
					},
				},
				{
					Type: "convert",
					Attributes: map[string]any{
						"field": "bytes",
						"type":  "long",
					},
					OnFailure: []*fleetpkg.Processor{
						{
							Type: "remove",
							Attributes: map[string]any{
								"field": "bytes",
							},
						},
					},
				},
			},
			basePath:  "/processors",
			wantCount: 3, // 2 regular + 1 on_failure
			validate: func(t *testing.T, result []FlatProcessor) {
				assert.Equal(t, "/processors/0", result[0].JSONPointer)
				assert.Equal(t, "/processors/1/on_failure/0", result[1].JSONPointer)
				assert.Equal(t, "/processors/1", result[2].JSONPointer)
			},
		},
		{
			name: "global on_failure processors",
			input: []*fleetpkg.Processor{
				{
					Type: "set",
					Attributes: map[string]any{
						"field": "error.global",
						"value": true,
					},
				},
			},
			basePath:  "/on_failure",
			wantCount: 1,
			validate: func(t *testing.T, result []FlatProcessor) {
				assert.Equal(t, "/on_failure/0", result[0].JSONPointer)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := FlattenProcessors(tt.input, tt.basePath)
			require.NoError(t, err)
			assert.Len(t, result, tt.wantCount)

			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestFlatProcessor_MarshalAttributes(t *testing.T) {
	tests := []struct {
		name       string
		processor  FlatProcessor
		wantEmpty  bool
		wantFields []string
	}{
		{
			name: "simple attributes",
			processor: FlatProcessor{
				Type: "set",
				Attributes: map[string]any{
					"field": "test",
					"value": 123,
				},
			},
			wantFields: []string{"field", "value"},
		},
		{
			name: "empty attributes",
			processor: FlatProcessor{
				Type:       "remove",
				Attributes: map[string]any{},
			},
			wantEmpty: true,
		},
		{
			name: "attributes with on_failure",
			processor: FlatProcessor{
				Type: "convert",
				Attributes: map[string]any{
					"field": "count",
					"type":  "long",
					"on_failure": []map[string]any{
						{
							"set": map[string]any{
								"field": "error",
								"value": "conversion_failed",
							},
						},
					},
				},
			},
			wantFields: []string{"field", "type", "on_failure"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.processor.MarshalAttributes()
			require.NoError(t, err)

			if tt.wantEmpty {
				assert.Empty(t, result)
				return
			}

			// Verify it's valid JSON
			var decoded map[string]any
			err = json.Unmarshal([]byte(result), &decoded)
			require.NoError(t, err)

			// Check expected fields are present
			for _, field := range tt.wantFields {
				assert.Contains(t, decoded, field)
			}
		})
	}
}
