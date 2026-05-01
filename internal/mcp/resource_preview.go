package mcp

import "fmt"

func truncateResourceText(content string, maxBytes int) string {
	return previewResourceText(content, 0, maxBytes, maxBytes)
}

func previewResourceText(content string, offsetBytes int, requestedBytes int, hardLimit int) string {
	if offsetBytes < 0 {
		offsetBytes = 0
	}

	limit := hardLimit
	if limit <= 0 {
		limit = requestedBytes
	}
	if requestedBytes > 0 && (limit <= 0 || requestedBytes < limit) {
		limit = requestedBytes
	}
	if limit <= 0 || (offsetBytes == 0 && len(content) <= limit) {
		if offsetBytes == 0 {
			return content
		}
		if offsetBytes >= len(content) {
			return "(no content at requested offset)"
		}
		return content[offsetBytes:]
	}
	if offsetBytes >= len(content) {
		return "(no content at requested offset)"
	}

	end := offsetBytes + limit
	if end > len(content) {
		end = len(content)
	}
	window := content[offsetBytes:end]
	if offsetBytes == 0 && end == len(content) {
		return window
	}

	remainingBefore := offsetBytes
	remainingAfter := len(content) - end
	result := window
	if remainingAfter > 0 {
		result += fmt.Sprintf("\n\n[mcp_resource truncated %d trailing bytes]", remainingAfter)
	}
	result += fmt.Sprintf("\n[mcp_resource showing bytes %d-%d of %d]", offsetBytes+1, end, len(content))
	if remainingBefore > 0 {
		result += fmt.Sprintf(" [skipped %d leading bytes]", remainingBefore)
	}
	return result
}
