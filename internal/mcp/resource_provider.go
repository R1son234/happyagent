package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func ReadResourceText(ctx context.Context, client *Client, uri string) (string, error) {
	result, err := client.session.ReadResource(ctx, &sdk.ReadResourceParams{URI: uri})
	if err != nil {
		return "", fmt.Errorf("read resource %q from server %q: %w", uri, client.name, err)
	}

	var parts []string
	for _, content := range result.Contents {
		if content == nil {
			continue
		}
		if content.Text != "" {
			parts = append(parts, content.Text)
			continue
		}
		if len(content.Blob) > 0 {
			parts = append(parts, base64.StdEncoding.EncodeToString(content.Blob))
		}
	}

	if len(parts) == 0 {
		payload, err := json.Marshal(result)
		if err != nil {
			return "", fmt.Errorf("marshal empty resource result for %q: %w", uri, err)
		}
		return string(payload), nil
	}

	return strings.Join(parts, "\n"), nil
}
