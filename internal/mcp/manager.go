package mcp

import (
	"context"
	"fmt"
	"sort"

	"happyagent/internal/config"
	"happyagent/internal/tools"
)

type ResourceInfo struct {
	ServerName  string `json:"server_name"`
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Manager struct {
	clients            map[string]*Client
	maxListedResources int
	resources          map[string]ResourceInfo
	tools              []tools.Tool
	maxResourceBytes   int
}

func NewManager(ctx context.Context, cfg config.MCPConfig) (*Manager, error) {
	manager := &Manager{
		clients:            make(map[string]*Client),
		maxListedResources: cfg.MaxListedResources,
		resources:          make(map[string]ResourceInfo),
		maxResourceBytes:   cfg.MaxResourceBytes,
	}

	for _, server := range cfg.Servers {
		if !server.Enabled {
			continue
		}

		client, err := NewClient(ctx, server)
		if err != nil {
			manager.Close()
			return nil, err
		}

		if _, exists := manager.clients[server.Name]; exists {
			manager.Close()
			return nil, fmt.Errorf("duplicate mcp server name %q", server.Name)
		}
		manager.clients[server.Name] = client

		if err := manager.loadTools(ctx, client); err != nil {
			manager.Close()
			return nil, err
		}
		if err := manager.loadResources(ctx, client); err != nil {
			manager.Close()
			return nil, err
		}
	}

	return manager, nil
}

func (m *Manager) loadTools(ctx context.Context, client *Client) error {
	result, err := client.session.ListTools(ctx, nil)
	if err != nil {
		return fmt.Errorf("list mcp tools from server %q: %w", client.name, err)
	}
	for _, tool := range result.Tools {
		adapter, err := NewToolAdapter(client, tool)
		if err != nil {
			return err
		}
		m.tools = append(m.tools, adapter)
	}
	return nil
}

func (m *Manager) loadResources(ctx context.Context, client *Client) error {
	result, err := client.session.ListResources(ctx, nil)
	if err != nil {
		return fmt.Errorf("list mcp resources from server %q: %w", client.name, err)
	}
	for _, resource := range result.Resources {
		m.resources[resource.URI] = ResourceInfo{
			ServerName:  client.name,
			URI:         resource.URI,
			Name:        resource.Name,
			Description: resource.Description,
		}
	}
	return nil
}

func (m *Manager) RegisterTools(registry *tools.Registry) ([]tools.Definition, error) {
	var defs []tools.Definition
	for _, tool := range m.tools {
		if err := registry.Register(tool); err != nil {
			return nil, err
		}
		defs = append(defs, tool.Definition())
	}
	return defs, nil
}

func (m *Manager) ReadResource(ctx context.Context, uri string) (string, error) {
	return m.ReadResourcePreview(ctx, uri, 0, 0)
}

func (m *Manager) ReadResourcePreview(ctx context.Context, uri string, offsetBytes int, maxBytes int) (string, error) {
	resource, ok := m.resources[uri]
	if !ok {
		return "", fmt.Errorf("mcp resource %q is not registered", uri)
	}
	client, ok := m.clients[resource.ServerName]
	if !ok {
		return "", fmt.Errorf("mcp server %q for resource %q is not connected", resource.ServerName, uri)
	}
	content, err := ReadResourceText(ctx, client, uri)
	if err != nil {
		return "", err
	}
	return previewResourceText(content, offsetBytes, maxBytes, m.maxResourceBytes), nil
}

func (m *Manager) ListResources() []ResourceInfo {
	out := make([]ResourceInfo, 0, len(m.resources))
	for _, resource := range m.resources {
		out = append(out, resource)
	}
	return out
}

func (m *Manager) ListResourcesPreview() ([]ResourceInfo, int, bool) {
	resources := m.ListResources()
	sort.Slice(resources, func(i, j int) bool {
		if resources[i].ServerName == resources[j].ServerName {
			return resources[i].URI < resources[j].URI
		}
		return resources[i].ServerName < resources[j].ServerName
	})
	total := len(resources)
	if m.maxListedResources <= 0 || total <= m.maxListedResources {
		return resources, total, false
	}
	return append([]ResourceInfo(nil), resources[:m.maxListedResources]...), total, true
}

func (m *Manager) HasResource(uri string) bool {
	_, ok := m.resources[uri]
	return ok
}

func (m *Manager) Close() error {
	var firstErr error
	for _, client := range m.clients {
		if err := client.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
