package httpapi

import (
	"encoding/json"
	"strings"
)

type agentToolRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type agentToolRPCResponse struct {
	JSONRPC string             `json:"jsonrpc"`
	ID      any                `json:"id"`
	Result  any                `json:"result,omitempty"`
	Error   *agentToolRPCError `json:"error,omitempty"`
}

type agentToolRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func agentToolProtocolVersion(params json.RawMessage) string {
	var payload struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(params, &payload)
	switch payload.ProtocolVersion {
	case "2024-11-05", "2025-03-26", "2025-06-18":
		return payload.ProtocolVersion
	default:
		return "2025-06-18"
	}
}
func defaultToolSchema(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return json.RawMessage(`{"type":"object"}`)
	}
	return raw
}
func mcpToolTextResult(text string, isError bool) map[string]any {
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"isError": isError,
	}
}
