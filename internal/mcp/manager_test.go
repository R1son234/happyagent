package mcp

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"happyagent/internal/config"
	"happyagent/internal/tools"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestManagerRegistersToolsAndReadsResources(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	manager, err := NewManager(ctx, config.MCPConfig{
		ConnectTimeoutSeconds: 5,
		Servers: []config.MCPServerConfig{
			helperServerConfig(t, "helper"),
		},
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer manager.Close()

	registry := tools.NewRegistry()
	defs, err := manager.RegisterTools(registry)
	if err != nil {
		t.Fatalf("RegisterTools() error = %v", err)
	}
	if len(defs) != 1 || defs[0].Name != "helper__repeat" {
		t.Fatalf("unexpected tool defs: %+v", defs)
	}

	result, err := registry.Execute(ctx, tools.Call{
		Name:      "helper__repeat",
		Arguments: []byte(`{"text":"hello"}`),
	})
	if err != nil {
		t.Fatalf("registry.Execute() error = %v", err)
	}
	if result.Output != "{\n  \"value\": \"mcpdemo:hello\"\n}" {
		t.Fatalf("unexpected tool output: %q", result.Output)
	}

	resource, err := manager.ReadResource(ctx, "demo://project-summary")
	if err != nil {
		t.Fatalf("ReadResource() error = %v", err)
	}
	if resource == "" {
		t.Fatalf("expected non-empty resource content")
	}
}

func TestMCPHelperProcess(t *testing.T) {
	if os.Getenv("HAPPYAGENT_MCP_HELPER") != "1" {
		return
	}

	server := sdk.NewServer(&sdk.Implementation{
		Name:    "helper",
		Version: "test",
	}, nil)

	sdk.AddTool(server, &sdk.Tool{
		Name:        "repeat",
		Description: "Repeat text for tests.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, input struct {
		Text string `json:"text"`
	}) (*sdk.CallToolResult, struct {
		Value string `json:"value"`
	}, error) {
		_ = ctx
		_ = req
		return nil, struct {
			Value string `json:"value"`
		}{Value: "mcpdemo:" + input.Text}, nil
	})

	server.AddResource(&sdk.Resource{
		Name:        "project-summary",
		URI:         "demo://project-summary",
		Description: "demo resource",
		MIMEType:    "text/plain",
	}, func(ctx context.Context, req *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
		_ = ctx
		_ = req
		return &sdk.ReadResourceResult{
			Contents: []*sdk.ResourceContents{
				{
					URI:      "demo://project-summary",
					MIMEType: "text/plain",
					Text:     "helper resource",
				},
			},
		}, nil
	})

	if err := server.Run(context.Background(), &sdk.StdioTransport{}); err != nil {
		t.Fatal(err)
	}
}

func helperServerConfig(t *testing.T, name string) config.MCPServerConfig {
	t.Helper()

	return config.MCPServerConfig{
		Name:    name,
		Command: os.Args[0],
		Args:    []string{"-test.run=TestMCPHelperProcess", "--"},
		Env: map[string]string{
			"HAPPYAGENT_MCP_HELPER": "1",
		},
		Enabled: true,
	}
}

func TestHelperServerConfigCommandExists(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestMCPHelperProcess", "--")
	if cmd.Path == "" {
		t.Fatal("expected helper command path")
	}
}
