package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jcpsimmons/room/internal/state"
)

type PruneOptions struct {
	WorkingDir string
	ConfigPath string
	Keep       int
	DryRun     bool
}

type PruneReport struct {
	RepoRoot            string   `json:"repo_root"`
	RunsDir             string   `json:"runs_dir"`
	Keep                int      `json:"keep"`
	DryRun              bool     `json:"dry_run"`
	Removed             []string `json:"removed,omitempty"`
	Kept                []string `json:"kept,omitempty"`
	RecoveryBundle      string   `json:"recovery_bundle,omitempty"`
	RecoveryBundleDrift int      `json:"recovery_bundle_drift,omitempty"`
	Lines               []string `json:"lines"`
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

	protectedBundles := map[string][]string{}
	if snapshot, err := state.Load(paths.StatePath); err == nil {
		addProtectedBundleReference(protectedBundles, snapshot.LastRunDirectory, "last run")
		addProtectedBundleReference(protectedBundles, snapshot.LastFailureRunDirectory, "last failure")
	}
	recoveryBundle, recoveryDrift, err := newestVerifiedRecoveryBundle(bundles)
	if err != nil {
		return PruneReport{}, err
	}
	if recoveryBundle != "" {
		addProtectedBundleReference(protectedBundles, recoveryBundle, "newest verified recovery bundle")
	}

	report := PruneReport{
		RepoRoot:            repoRoot,
		RunsDir:             paths.RunsDir,
		Keep:                opts.Keep,
		DryRun:              opts.DryRun,
		RecoveryBundle:      recoveryBundle,
		RecoveryBundleDrift: recoveryDrift,
	}

	lines := []string{
		"ROOM prune",
		fmt.Sprintf("Repo: %s", repoRoot),
		fmt.Sprintf("Runs dir: %s", paths.RunsDir),
		fmt.Sprintf("Keeping newest %d bundle(s).", opts.Keep),
	}
	if recoveryBundle != "" {
		lines = append(lines, fmt.Sprintf("Recovery anchor: keeping %s because %d newer bundle(s) drifted out of tune.", recoveryBundle, recoveryDrift))
	}

	if len(bundles) == 0 {
		lines = append(lines, "No ROOM run bundles found.")
		report.Lines = lines
		return report, nil
	}

	kept := make([]string, 0, opts.Keep+1)
	var removals []runBundle
	for i, bundle := range bundles {
		if i < opts.Keep {
			kept = append(kept, bundle.path)
			continue
		}
		if _, ok := protectedBundles[filepath.Clean(bundle.path)]; ok {
			kept = append(kept, bundle.path)
			continue
		}
		removals = append(removals, bundle)
	}
	report.Kept = kept

	action := "removed"
	if opts.DryRun {
		action = "would remove"
	}
	if len(removals) == 0 {
		if len(protectedBundles) > 0 {
			lines = append(lines, "Nothing to prune; state.json keeps an older bundle alive.")
		} else {
			lines = append(lines, "Nothing to prune.")
		}
		for _, kept := range report.Kept {
			if reasons, ok := protectedBundles[filepath.Clean(kept)]; ok {
				lines = append(lines, "kept "+kept+formatStateReferenceSuffix(reasons))
				continue
			}
			lines = append(lines, "kept "+kept)
		}
		report.Lines = lines
		return report, nil
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
		if reasons, ok := protectedBundles[filepath.Clean(kept)]; ok {
			lines = append(lines, "kept "+kept+formatStateReferenceSuffix(reasons))
			continue
		}
		lines = append(lines, "kept "+kept)
	}
	report.Lines = lines
	return report, nil
}

func addProtectedBundleReference(protected map[string][]string, bundleDir, reason string) {
	bundleDir = filepath.Clean(strings.TrimSpace(bundleDir))
	if bundleDir == "." || bundleDir == "" {
		return
	}
	reasons := protected[bundleDir]
	for _, existing := range reasons {
		if existing == reason {
			return
		}
	}
	protected[bundleDir] = append(reasons, reason)
}

func formatStateReferenceSuffix(reasons []string) string {
	if len(reasons) == 0 {
		return ""
	}
	return fmt.Sprintf(" (referenced by state.json %s)", strings.Join(reasons, " and "))
}

func newestVerifiedRecoveryBundle(bundles []runBundle) (string, int, error) {
	drifted := 0
	for _, bundle := range bundles {
		assessment, err := assessBundle(bundle.path)
		if err != nil {
			return "", 0, err
		}
		if assessment.Integrity == bundleIntegrityOK {
			if drifted == 0 {
				return "", 0, nil
			}
			return bundle.path, drifted, nil
		}
		if bundleShowsRecoveryDrift(assessment) {
			drifted++
			continue
		}
		return "", 0, nil
	}
	return "", 0, nil
}

func bundleShowsRecoveryDrift(assessment bundleAssessment) bool {
	if assessment.Integrity == bundleIntegrityOK {
		return false
	}
	return strings.TrimSpace(assessment.Hint) != "" || len(assessment.Hints) > 0
}
