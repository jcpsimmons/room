package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func gitInfoExcludePath(repoRoot string) (string, error) {
	gitPath := filepath.Join(repoRoot, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return filepath.Join(gitPath, "info", "exclude"), nil
	}

	data, err := os.ReadFile(gitPath)
	if err != nil {
		return "", err
	}
	gitDir, err := parseGitDirPointer(repoRoot, string(data))
	if err != nil {
		return "", err
	}

	commonDir, err := resolveGitCommonDir(gitDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(commonDir, "info", "exclude"), nil
}

func parseGitDirPointer(repoRoot, raw string) (string, error) {
	line := strings.TrimSpace(raw)
	if !strings.HasPrefix(line, "gitdir:") {
		return "", fmt.Errorf("unsupported .git indirection in %s", filepath.Join(repoRoot, ".git"))
	}
	target := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
	if target == "" {
		return "", fmt.Errorf("missing gitdir target in %s", filepath.Join(repoRoot, ".git"))
	}
	if filepath.IsAbs(target) {
		return filepath.Clean(target), nil
	}
	return filepath.Clean(filepath.Join(repoRoot, target)), nil
}

func resolveGitCommonDir(gitDir string) (string, error) {
	commonDirPath := filepath.Join(gitDir, "commondir")
	data, err := os.ReadFile(commonDirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return gitDir, nil
		}
		return "", err
	}
	target := strings.TrimSpace(string(data))
	if target == "" {
		return "", fmt.Errorf("empty commondir in %s", commonDirPath)
	}
	if filepath.IsAbs(target) {
		return filepath.Clean(target), nil
	}
	return filepath.Clean(filepath.Join(gitDir, target)), nil
}
