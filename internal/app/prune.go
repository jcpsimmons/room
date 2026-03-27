package app

import (
	"context"
	"errors"
	"fmt"
	"os"
)

type PruneOptions struct {
	WorkingDir string
	ConfigPath string
	Keep       int
	DryRun     bool
}

type PruneReport struct {
	RepoRoot string   `json:"repo_root"`
	RunsDir  string   `json:"runs_dir"`
	Keep     int      `json:"keep"`
	DryRun   bool     `json:"dry_run"`
	Removed  []string `json:"removed,omitempty"`
	Kept     []string `json:"kept,omitempty"`
	Lines    []string `json:"lines"`
}

func (s *Service) Prune(ctx context.Context, opts PruneOptions) (PruneReport, error) {
	repoRoot, err := s.requireRepo(ctx, opts.WorkingDir)
	if err != nil {
		return PruneReport{}, err
	}
	_, paths, err := s.loadConfig(ctx, repoRoot, opts.ConfigPath)
	if err != nil {
		return PruneReport{}, err
	}

	if opts.Keep < 1 {
		return PruneReport{}, errors.New("keep must be at least 1")
	}

	bundles, err := runBundles(paths.RunsDir)
	if err != nil {
		return PruneReport{}, err
	}

	report := PruneReport{
		RepoRoot: repoRoot,
		RunsDir:  paths.RunsDir,
		Keep:     opts.Keep,
		DryRun:   opts.DryRun,
	}

	lines := []string{
		"ROOM prune",
		fmt.Sprintf("Repo: %s", repoRoot),
		fmt.Sprintf("Runs dir: %s", paths.RunsDir),
		fmt.Sprintf("Keeping newest %d bundle(s).", opts.Keep),
	}

	if len(bundles) == 0 {
		lines = append(lines, "No ROOM run bundles found.")
		report.Lines = lines
		return report, nil
	}

	if len(bundles) <= opts.Keep {
		for _, bundle := range bundles {
			report.Kept = append(report.Kept, bundle.path)
		}
		lines = append(lines, "Nothing to prune.")
		for _, kept := range report.Kept {
			lines = append(lines, "kept "+kept)
		}
		report.Lines = lines
		return report, nil
	}

	report.Kept = make([]string, 0, opts.Keep)
	for _, bundle := range bundles[:opts.Keep] {
		report.Kept = append(report.Kept, bundle.path)
	}

	removals := bundles[opts.Keep:]
	action := "removed"
	if opts.DryRun {
		action = "would remove"
	}
	lines = append(lines, fmt.Sprintf("%s %d older bundle(s).", action, len(removals)))
	for _, bundle := range removals {
		if !opts.DryRun {
			if err := os.RemoveAll(bundle.path); err != nil {
				return PruneReport{}, err
			}
		}
		report.Removed = append(report.Removed, bundle.path)
		lines = append(lines, fmt.Sprintf("%s %s", action, bundle.path))
	}
	if opts.DryRun {
		lines = append(lines, "Dry run only; nothing was deleted.")
	}
	for _, kept := range report.Kept {
		lines = append(lines, "kept "+kept)
	}
	report.Lines = lines
	return report, nil
}
