package claude

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/jcpsimmons/room/internal/agent"
)

type outputEnvelope struct {
	IsError          bool            `json:"is_error"`
	Result           string          `json:"result"`
	StructuredOutput json.RawMessage `json:"structured_output"`
}

func ParseOutput(raw []byte) (agent.Result, error) {
	envelope, err := parseOutputEnvelope(raw)
	if err != nil {
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

func parseOutputEnvelope(raw []byte) (outputEnvelope, error) {
	var envelope outputEnvelope
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return outputEnvelope{}, fmt.Errorf("malformed claude output envelope: %w", err)
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return outputEnvelope{}, fmt.Errorf("malformed claude output envelope: unexpected trailing data")
		}
		return outputEnvelope{}, fmt.Errorf("malformed claude output envelope: %w", err)
	}
	return envelope, nil
}
