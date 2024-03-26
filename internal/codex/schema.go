package codex

import "github.com/jcpsimmons/room/internal/agent"

type Schema = agent.Schema

func DefaultSchema() []byte {
	return agent.DefaultSchema()
}

func WriteSchema(path string) error {
	return agent.WriteSchema(path)
}
