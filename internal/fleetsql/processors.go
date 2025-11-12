// Licensed to Elasticsearch B.V. under one or more agreements.
// Elasticsearch B.V. licenses this file to you under the Apache 2.0 License.
// See the LICENSE file in the project root for more information.

package fleetsql

import (
	"encoding/json"
	"fmt"

	"github.com/andrewkroh/go-fleetpkg"
)

// FlatProcessor represents a flattened processor with its JSON Pointer location.
type FlatProcessor struct {
	Type        string
	Attributes  map[string]any
	JSONPointer string
	FilePath    string
	Line        int
	Column      int
}

// FlattenProcessors recursively flattens a list of processors, computing JSON Pointers
// and merging on_failure handlers into the attributes.
func FlattenProcessors(processors []*fleetpkg.Processor, basePath string) ([]FlatProcessor, error) {
	var result []FlatProcessor

	for i, p := range processors {
		if p == nil {
			continue
		}

		// Compute JSON Pointer for this processor
		jsonPointer := fmt.Sprintf("%s/%d/%s", basePath, i, p.Type)

		// Create a copy of attributes and add on_failure if present
		attrs := make(map[string]any)
		for k, v := range p.Attributes {
			attrs[k] = v
		}

		// Process on_failure handlers
		if len(p.OnFailure) > 0 {
			// Recursively flatten on_failure processors
			onFailureFlat, err := FlattenProcessors(p.OnFailure, jsonPointer+"/on_failure")
			if err != nil {
				return nil, err
			}
			result = append(result, onFailureFlat...)

			// Marshal on_failure for this processor's attributes
			onFailureJSON := make([]map[string]any, 0, len(p.OnFailure))
			for _, of := range p.OnFailure {
				procAttrs := make(map[string]any)
				procAttrs[of.Type] = of.Attributes
				onFailureJSON = append(onFailureJSON, procAttrs)
			}
			attrs["on_failure"] = onFailureJSON
		}

		// Add this processor
		result = append(result, FlatProcessor{
			Type:        p.Type,
			Attributes:  attrs,
			JSONPointer: jsonPointer,
			FilePath:    p.Path(),
			Line:        p.Line(),
			Column:      p.Column(),
		})
	}

	return result, nil
}

// MarshalAttributes marshals the processor attributes to JSON.
func (fp FlatProcessor) MarshalAttributes() (string, error) {
	if len(fp.Attributes) == 0 {
		return "", nil
	}
	data, err := json.Marshal(fp.Attributes)
	if err != nil {
		return "", fmt.Errorf("failed to marshal processor attributes: %w", err)
	}
	return string(data), nil
}
