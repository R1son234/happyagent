package mcp

import (
	"context"
	"os"
	"os/exec"
	"strings"
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
		MaxResourceBytes:      256,
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

func TestManagerTruncatesLargeResources(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	manager, err := NewManager(ctx, config.MCPConfig{
		ConnectTimeoutSeconds: 5,
		MaxResourceBytes:      64,
		Servers: []config.MCPServerConfig{
			helperServerConfig(t, "helper"),
		},
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer manager.Close()

	resource, err := manager.ReadResource(ctx, "demo://large-resource")
	if err != nil {
		t.Fatalf("ReadResource() error = %v", err)
	}
	if resource == "" {
		t.Fatal("expected truncated resource content")
	}
	if !strings.Contains(resource, "[mcp_resource truncated") {
		t.Fatalf("expected truncation marker, got %q", resource)
	}
}

func TestManagerResourcePreviewSupportsOffsets(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	manager, err := NewManager(ctx, config.MCPConfig{
		ConnectTimeoutSeconds: 5,
		MaxListedResources:    10,
		MaxResourceBytes:      64,
		Servers: []config.MCPServerConfig{
			helperServerConfig(t, "helper"),
		},
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer manager.Close()

	resource, err := manager.ReadResourcePreview(ctx, "demo://large-resource", 10, 16)
	if err != nil {
		t.Fatalf("ReadResourcePreview() error = %v", err)
	}
	if resource == "" {
		t.Fatal("expected preview content")
	}
	if !strings.Contains(resource, "[mcp_resource showing bytes") {
		t.Fatalf("expected preview marker, got %q", resource)
	}
}

func TestManagerListResourcesPreviewTruncatesCount(t *testing.T) {
	manager := &Manager{
		maxListedResources: 1,
		resources: map[string]ResourceInfo{
			"demo://b": {ServerName: "demo", URI: "demo://b", Name: "b"},
			"demo://a": {ServerName: "demo", URI: "demo://a", Name: "a"},
		},
	}

	resources, total, truncated := manager.ListResourcesPreview()
	if total != 2 {
		t.Fatalf("unexpected total: %d", total)
	}
	if !truncated {
		t.Fatal("expected truncated preview")
	}
	if len(resources) != 1 || resources[0].URI != "demo://a" {
		t.Fatalf("unexpected preview resources: %+v", resources)
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

	server.AddResource(&sdk.Resource{
		Name:        "large-resource",
		URI:         "demo://large-resource",
		Description: "large demo resource",
		MIMEType:    "text/plain",
	}, func(ctx context.Context, req *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
		_ = ctx
		_ = req
		return &sdk.ReadResourceResult{
			Contents: []*sdk.ResourceContents{
				{
					URI:      "demo://large-resource",
					MIMEType: "text/plain",
					Text:     "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789",
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
