package claude

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/jcpsimmons/room/internal/agent"
)

type outputEnvelope struct {
	IsError          bool            `json:"is_error"`
	Result           string          `json:"result"`
	StructuredOutput json.RawMessage `json:"structured_output"`
}

func ParseOutput(raw []byte) (agent.Result, error) {
	var envelope outputEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return agent.Result{}, err
	}
	if envelope.IsError {
		message := strings.TrimSpace(envelope.Result)
		if message == "" {
			message = "claude execution failed"
		}
		return agent.Result{}, errors.New(message)
	}
	if len(envelope.StructuredOutput) > 0 && string(envelope.StructuredOutput) != "null" {
		return agent.ParseResult(envelope.StructuredOutput)
	}
	if strings.TrimSpace(envelope.Result) != "" {
		return agent.ParseResult([]byte(envelope.Result))
	}
	return agent.Result{}, errors.New("claude did not return structured output")
}
