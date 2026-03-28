package app

import (
	"fmt"
	"strings"

	"github.com/jcpsimmons/room/internal/state"
)

type instructionDriftSignal struct {
	Detected    bool
	Message     string
	CurrentHash string
}

func inspectInstructionDrift(snapshot state.Snapshot, instruction instructionSignal) instructionDriftSignal {
	if strings.TrimSpace(instruction.Hint) != "" || strings.TrimSpace(instruction.Body) == "" {
		return instructionDriftSignal{}
	}

	currentHash := state.InstructionHash(instruction.Body)
	recordedHash := strings.TrimSpace(snapshot.CurrentInstructionHash)
	if recordedHash == "" || recordedHash == currentHash {
		return instructionDriftSignal{CurrentHash: currentHash}
	}

	message := "Instruction drift: state.json no longer matches instruction.txt; ROOM will re-anchor to the live instruction on the next run."
	if last := strings.TrimSpace(snapshot.LastNextInstruction); last != "" && last != strings.TrimSpace(instruction.Body) {
		message = fmt.Sprintf("%s Last recorded next instruction diverged from the live file.", message)
	}

	return instructionDriftSignal{
		Detected:    true,
		Message:     message,
		CurrentHash: currentHash,
	}
}
