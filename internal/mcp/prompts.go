// Licensed to Elasticsearch B.V. under one or more agreements.
// Elasticsearch B.V. licenses this file to you under the Apache 2.0 License.
// See the LICENSE file in the project root for more information.

package mcp

import (
	"context"
	_ "embed"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

//go:embed fleetpkg_guide.md
var guidePromptText string

// AddPrompts registers all MCP prompts on the server.
func AddPrompts(s *mcp.Server) {
	s.AddPrompt(&mcp.Prompt{
		Name:        "fleetpkg_guide",
		Title:       "Fleet Integration Package Guide",
		Description: "How to use the fleetpkg tools together to explore Elastic integration packages",
	}, guidePromptHandler)
}

func guidePromptHandler(_ context.Context, _ *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	return &mcp.GetPromptResult{
		Description: "Guide for using the fleetpkg MCP tools",
		Messages: []*mcp.PromptMessage{
			{
				Role:    "user",
				Content: &mcp.TextContent{Text: guidePromptText},
			},
		},
	}, nil
}
