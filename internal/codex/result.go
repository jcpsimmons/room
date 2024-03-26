package codex

import "github.com/jcpsimmons/room/internal/agent"

type Result = agent.Result
type Execution = agent.Execution

func ParseResult(raw []byte) (Result, error) {
	return agent.ParseResult(raw)
}
