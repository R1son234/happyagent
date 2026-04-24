package main

import (
	"context"
	"fmt"
	"log"

	mcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type RepeatInput struct {
	Text string `json:"text"`
}

type RepeatOutput struct {
	Value string `json:"value"`
}

func main() {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "happyagent-mcpdemo",
		Version: "0.1.0",
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "repeat",
		Description: "Repeat the given text with an mcpdemo prefix.",
	}, repeat)

	server.AddResource(&mcp.Resource{
		Name:        "project-summary",
		URI:         "demo://project-summary",
		Description: "A short summary of the happyagent demo project.",
		MIMEType:    "text/plain",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		_ = ctx
		_ = req
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{
					URI:      "demo://project-summary",
					MIMEType: "text/plain",
					Text:     "happyagent demo MCP server exposes one repeat tool and one project summary resource.",
				},
			},
		}, nil
	})

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}

func repeat(ctx context.Context, req *mcp.CallToolRequest, input RepeatInput) (*mcp.CallToolResult, RepeatOutput, error) {
	_ = ctx
	_ = req
	if input.Text == "" {
		return nil, RepeatOutput{}, fmt.Errorf("text must not be empty")
	}
	return nil, RepeatOutput{Value: "mcpdemo:" + input.Text}, nil
}
