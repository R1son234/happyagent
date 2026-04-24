package mcp

import (
	"context"
	"fmt"
	"os/exec"

	"happyagent/internal/config"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type Client struct {
	name    string
	session *sdk.ClientSession
}

func NewClient(ctx context.Context, cfg config.MCPServerConfig) (*Client, error) {
	command := exec.CommandContext(ctx, cfg.Command, cfg.Args...)
	if len(cfg.Env) > 0 {
		env := command.Environ()
		for key, value := range cfg.Env {
			env = append(env, key+"="+value)
		}
		command.Env = env
	}

	client := sdk.NewClient(&sdk.Implementation{
		Name:    "happyagent",
		Version: "0.1.0",
	}, nil)

	session, err := client.Connect(ctx, &sdk.CommandTransport{Command: command}, nil)
	if err != nil {
		return nil, fmt.Errorf("connect to mcp server %q: %w", cfg.Name, err)
	}

	return &Client{
		name:    cfg.Name,
		session: session,
	}, nil
}

func (c *Client) Close() error {
	if c == nil || c.session == nil {
		return nil
	}
	return c.session.Close()
}
