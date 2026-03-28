package app

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

type BundleOptions struct {
	WorkingDir string
	ConfigPath string
	RunDir     string
}

type BundleArtifactReport struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type BundleReport struct {
	RepoRoot             string                 `json:"repo_root"`
	RunsDir              string                 `json:"runs_dir"`
	RunDir               string                 `json:"run_dir"`
	BundleMode           string                 `json:"bundle_mode,omitempty"`
	BundleIntegrity      string                 `json:"bundle_integrity,omitempty"`
	BundleHint           string                 `json:"bundle_hint,omitempty"`
	BundleIntegrityHints []BundleIntegrityHint  `json:"bundle_integrity_hints,omitempty"`
	Execution            *ExecutionReport       `json:"execution,omitempty"`
	Progress             *ProgressReport        `json:"progress,omitempty"`
	ManifestOK           bool                   `json:"manifest_ok"`
	Artifacts            []BundleArtifactReport `json:"artifacts,omitempty"`
	Lines                []string               `json:"lines"`
}

func (s *Service) Bundle(ctx context.Context, opts BundleOptions) (BundleReport, error) {
	repoRoot, err := s.requireRepo(ctx, opts.WorkingDir)
	if err != nil {
		return BundleReport{}, err
	}
	_, paths, err := s.loadConfig(ctx, repoRoot, opts.ConfigPath)
	if err != nil {
		return BundleReport{}, err
	}

	runDir := opts.RunDir
	if runDir == "" {
		runDir, err = latestRunBundle(paths.RunsDir)
		if err != nil {
			if strings.HasPrefix(err.Error(), "no ROOM run bundles found in ") {
				return BundleReport{
					RepoRoot: repoRoot,
					RunsDir:  paths.RunsDir,
					Lines: []string{
						"ROOM bundle",
						fmt.Sprintf("Repo: %s", repoRoot),
						fmt.Sprintf("Runs dir: %s", paths.RunsDir),
						"No ROOM run bundles found.",
					},
				}, nil
			}
			return BundleReport{}, err
		}
	} else if !filepath.IsAbs(runDir) {
		runDir = filepath.Join(paths.RunsDir, runDir)
	}

	assessment, err := assessBundle(runDir)
	if err != nil {
		return BundleReport{}, err
	}

	manifest, manifestOK, _, err := readBundleManifest(runDir)
	if err != nil {
		return BundleReport{}, err
	}
	execution, hasExecution, executionWarn, err := readExecutionArtifactLenient(filepath.Join(runDir, "execution.json"))
	if err != nil {
		return BundleReport{}, err
	}
	if executionWarn != nil {
		appendArtifactDecodeWarning(&assessment, "bundle", "execution.json", executionWarn)
	}
	progress, hasProgress, progressWarn, err := readProgressArtifactLenient(filepath.Join(runDir, "progress.jsonl"))
	if err != nil {
		return BundleReport{}, err
	}
	if progressWarn != nil {
		appendArtifactDecodeWarning(&assessment, "bundle", "progress.jsonl", progressWarn)
	}

	report := BundleReport{
		RepoRoot:             repoRoot,
		RunsDir:              paths.RunsDir,
		RunDir:               runDir,
		BundleMode:           string(assessment.Mode),
		BundleIntegrity:      assessment.Integrity,
		BundleHint:           assessment.Hint,
		BundleIntegrityHints: assessment.Hints,
		Execution:            executionReportIfPresent(execution, hasExecution),
		Progress:             progressReportIfPresent(progress, hasProgress),
		ManifestOK:           manifestOK && assessment.ManifestOK,
	}

	if manifestOK {
		report.Artifacts = make([]BundleArtifactReport, 0, len(manifest.Artifacts))
		for _, artifact := range manifest.Artifacts {
			report.Artifacts = append(report.Artifacts, BundleArtifactReport{
				Name:   artifact.Name,
				Size:   artifact.Size,
				SHA256: artifact.SHA256,
			})
		}
		sort.Slice(report.Artifacts, func(i, j int) bool {
			return report.Artifacts[i].Name < report.Artifacts[j].Name
		})
	}

	lines := []string{
		"ROOM bundle",
		fmt.Sprintf("Repo: %s", repoRoot),
		fmt.Sprintf("Runs dir: %s", paths.RunsDir),
		fmt.Sprintf("Bundle dir: %s", runDir),
		fmt.Sprintf("Bundle mode: %s", report.BundleMode),
		fmt.Sprintf("Bundle integrity: %s", report.BundleIntegrity),
	}
	if report.BundleHint != "" {
		lines = append(lines, report.BundleHint)
	}
	if len(report.BundleIntegrityHints) > 0 {
		lines = append(lines, fmt.Sprintf("Bundle integrity hints: %s", manifestHintsJSON(report.BundleIntegrityHints)))
	}
	lines = append(lines, executionLines(execution, hasExecution)...)
	lines = append(lines, progressLines(progress, hasProgress)...)
	if manifestOK {
		lines = append(lines, "Manifest artifacts:")
		for _, artifact := range report.Artifacts {
			lines = append(lines, indent(fmt.Sprintf("%s (%d bytes, %s)", artifact.Name, artifact.Size, shortSHA(artifact.SHA256))))
		}
	} else {
		lines = append(lines, "Manifest artifacts: unavailable")
	}

	report.Lines = lines
	return report, nil
}

func shortSHA(sum string) string {
	sum = strings.TrimSpace(sum)
	if len(sum) <= 8 {
		return sum
	}
	return sum[:8]
}
