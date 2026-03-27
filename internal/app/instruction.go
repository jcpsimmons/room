package app

import (
	"fmt"
	"strings"

	"github.com/jcpsimmons/room/internal/fsutil"
)

type instructionSignal struct {
	Body string
	Hint string
}

func loadInstructionSignal(path string) (instructionSignal, error) {
	data, err := fsutil.ReadFileIfExists(path)
	if err != nil {
		return instructionSignal{}, err
	}
	if !fsutil.FileExists(path) {
		return instructionSignal{
			Hint: "Current instruction unavailable: missing instruction.txt.",
		}, nil
	}

	body := strings.TrimSpace(string(data))
	if body == "" {
		return instructionSignal{
			Hint: "Current instruction unavailable: instruction.txt is blank.",
		}, nil
	}

	return instructionSignal{Body: body}, nil
}

func requireInstructionSignal(path string) (string, error) {
	signal, err := loadInstructionSignal(path)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(signal.Hint) != "" {
		return "", fmt.Errorf("%s Seed a new prompt before running `room run`.", signal.Hint)
	}
	return signal.Body, nil
}
