package codex

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/jcpsimmons/room/internal/agent"
)

var errMalformedJSON = errors.New("malformed ROOM JSON result")

type Result = agent.Result
type Execution = agent.Execution

func ParseResult(raw []byte) (Result, error) {
	payload, err := extractJSONObject(raw)
	if err != nil {
		return Result{}, malformedResultError(raw, err)
	}
	return agent.ParseResult(payload)
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

func malformedResultError(raw []byte, err error) error {
	return fmt.Errorf("%w: %v (input %q)", errMalformedJSON, err, previewJSON(raw))
}

func previewJSON(raw []byte) string {
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
