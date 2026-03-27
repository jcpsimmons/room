package agent

import "errors"

func ExtractJSONObject(raw []byte) ([]byte, error) {
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

func extractJSONObject(raw []byte) ([]byte, error) {
	return ExtractJSONObject(raw)
}
