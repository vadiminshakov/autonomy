package client

import (
	"github.com/modelcontextprotocol/go-sdk/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vadiminshakov/autonomy/core/entity"
)

// ConvertToMCPTools converts our ToolDefinition to MCP tools
func ConvertToMCPTools(tools []entity.ToolDefinition) []*mcp.Tool {
	mcpTools := make([]*mcp.Tool, len(tools))

	for i, tool := range tools {
		var inputSchema *jsonschema.Schema
		if tool.InputSchema != nil {
			inputSchema = &jsonschema.Schema{
				Type: "object",
			}
		}

		mcpTools[i] = &mcp.Tool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: inputSchema,
		}
	}

	return mcpTools
}
