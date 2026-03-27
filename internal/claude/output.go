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

var ErrMalformedOutputEnvelope = errors.New("malformed claude output envelope")

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
	payload, err := extractJSONObject(raw)
	if err != nil {
		return outputEnvelope{}, malformedOutputEnvelopeError(raw, err)
	}

	var envelope outputEnvelope
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return outputEnvelope{}, malformedOutputEnvelopeError(raw, err)
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return outputEnvelope{}, malformedOutputEnvelopeError(raw, errors.New("unexpected trailing data"))
		}
		return outputEnvelope{}, malformedOutputEnvelopeError(raw, err)
	}
	return envelope, nil
}

func extractJSONObject(raw []byte) ([]byte, error) {
	for start := 0; start < len(raw); start++ {
		if raw[start] != '{' {
			continue
		}

		depth := 0
		inString := false
		escaped := false
		started := false

		for i := start; i < len(raw); i++ {
			b := raw[i]
			if !started {
				started = true
				depth = 1
				continue
			}

			if inString {
				if escaped {
					escaped = false
					continue
				}
				switch b {
				case '\\':
					escaped = true
				case '"':
					inString = false
				}
				continue
			}

			switch b {
			case '"':
				inString = true
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					return raw[start : i+1], nil
				}
			}
		}
	}

	return nil, errors.New("could not find a complete JSON object")
}

func malformedOutputEnvelopeError(raw []byte, err error) error {
	return fmt.Errorf("%w: %v (input %q)", ErrMalformedOutputEnvelope, err, previewOutputJSON(raw))
}

func previewOutputJSON(raw []byte) string {
	const limit = 160
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return ""
	}
	buf := make([]byte, 0, len(trimmed))
	for _, b := range trimmed {
		switch b {
		case '\n', '\r', '\t':
			b = ' '
		}
		if b < 0x20 {
			b = ' '
		}
		buf = append(buf, b)
		if len(buf) >= limit {
			break
		}
	}
	if len(trimmed) > len(buf) {
		return string(buf) + "..."
	}
	return string(buf)
}
