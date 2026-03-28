package app

import (
	"bytes"
	"fmt"

	"github.com/jcpsimmons/room/internal/agent"
	"github.com/jcpsimmons/room/internal/fsutil"
)

type schemaContractStatus string

const (
	schemaContractCurrent schemaContractStatus = "current"
	schemaContractMissing schemaContractStatus = "missing"
	schemaContractStale   schemaContractStatus = "stale"
)

type schemaContractSignal struct {
	Status schemaContractStatus
}

func inspectSchemaContract(path string) (schemaContractSignal, error) {
	data, err := fsutil.ReadFileIfExists(path)
	if err != nil {
		return schemaContractSignal{}, err
	}
	if len(data) == 0 {
		return schemaContractSignal{Status: schemaContractMissing}, nil
	}
	if bytes.Equal(data, agent.DefaultSchema()) {
		return schemaContractSignal{Status: schemaContractCurrent}, nil
	}
	return schemaContractSignal{Status: schemaContractStale}, nil
}

func syncSchemaContract(path string) (schemaContractSignal, error) {
	signal, err := inspectSchemaContract(path)
	if err != nil {
		return schemaContractSignal{}, err
	}
	switch signal.Status {
	case schemaContractMissing, schemaContractStale:
		if err := agent.WriteSchema(path); err != nil {
			return schemaContractSignal{}, err
		}
		return schemaContractSignal{Status: schemaContractCurrent}, nil
	default:
		return signal, nil
	}
}

func schemaContractHint(signal schemaContractSignal, path string) string {
	switch signal.Status {
	case schemaContractCurrent:
		return fmt.Sprintf("schema contract is current: %s", path)
	case schemaContractMissing:
		return fmt.Sprintf("schema contract is missing at %s; the next init or run will regenerate it", path)
	case schemaContractStale:
		return fmt.Sprintf("schema contract at %s drifted from this ROOM build; the next init or run will refresh it", path)
	default:
		return ""
	}
}
