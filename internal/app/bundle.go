package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jcpsimmons/room/internal/fsutil"
)

type bundleMode string

const (
	bundleModeDryRun    bundleMode = "dry_run"
	bundleModeExecuted  bundleMode = "executed"
	bundleModeLegacy    bundleMode = "legacy"
	bundleIntegrityOK              = "verified"
	bundleIntegrityWarn            = "unverified"
	bundleIntegrityBad             = "mismatch"
)

var errMalformedBundleManifest = fmt.Errorf("malformed bundle manifest")

type bundleArtifact struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type bundleManifest struct {
	RunDir    string           `json:"run_dir"`
	Mode      bundleMode       `json:"mode"`
	CreatedAt time.Time        `json:"created_at"`
	Artifacts []bundleArtifact `json:"artifacts"`
}

type bundleAssessment struct {
	RunDir     string
	Mode       bundleMode
	Integrity  string
	Hint       string
	ManifestOK bool
}

func writeBundleManifest(runDir string, mode bundleMode, artifactNames []string) error {
	manifest := bundleManifest{
		RunDir:    runDir,
		Mode:      mode,
		CreatedAt: time.Now().UTC(),
	}
	for _, name := range artifactNames {
		artifactPath := filepath.Join(runDir, name)
		data, err := os.ReadFile(artifactPath)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		manifest.Artifacts = append(manifest.Artifacts, bundleArtifact{
			Name:   name,
			Size:   int64(len(data)),
			SHA256: hex.EncodeToString(sum[:]),
		})
	}
	sort.Slice(manifest.Artifacts, func(i, j int) bool {
		return manifest.Artifacts[i].Name < manifest.Artifacts[j].Name
	})

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return fsutil.AtomicWriteFile(filepath.Join(runDir, "bundle.json"), data, 0o644)
}

func readBundleManifest(runDir string) (bundleManifest, bool, error) {
	data, err := fsutil.ReadFileIfExists(filepath.Join(runDir, "bundle.json"))
	if err != nil {
		return bundleManifest{}, false, err
	}
	if len(data) == 0 {
		return bundleManifest{}, false, nil
	}

	var manifest bundleManifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return bundleManifest{}, false, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return bundleManifest{}, false, fmt.Errorf("%w: unexpected trailing data", errMalformedBundleManifest)
		}
		return bundleManifest{}, false, err
	}
	if err := manifest.validate(); err != nil {
		return bundleManifest{}, false, fmt.Errorf("%w: %v", errMalformedBundleManifest, err)
	}
	return manifest, true, nil
}

func (manifest bundleManifest) validate() error {
	if strings.TrimSpace(manifest.RunDir) == "" {
		return fmt.Errorf("missing run_dir")
	}
	switch manifest.Mode {
	case bundleModeDryRun, bundleModeExecuted, bundleModeLegacy:
	default:
		return fmt.Errorf("invalid mode %q", manifest.Mode)
	}
	seen := make(map[string]struct{}, len(manifest.Artifacts))
	for _, artifact := range manifest.Artifacts {
		if strings.TrimSpace(artifact.Name) == "" {
			return fmt.Errorf("artifact name missing")
		}
		if _, ok := seen[artifact.Name]; ok {
			return fmt.Errorf("duplicate artifact %q", artifact.Name)
		}
		seen[artifact.Name] = struct{}{}
		if len(artifact.SHA256) != 64 {
			return fmt.Errorf("artifact %q has malformed sha256", artifact.Name)
		}
		for _, r := range artifact.SHA256 {
			if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
				continue
			}
			return fmt.Errorf("artifact %q has malformed sha256", artifact.Name)
		}
	}
	return nil
}

func assessNewestBundle(runsDir string) (bundleAssessment, error) {
	latestRunDir, err := latestRunBundle(runsDir)
	if err != nil {
		if strings.HasPrefix(err.Error(), "no ROOM run bundles found in ") {
			return bundleAssessment{}, nil
		}
		return bundleAssessment{}, err
	}
	return assessBundleNamed(latestRunDir, "newest bundle")
}

func assessBundle(runDir string) (bundleAssessment, error) {
	return assessBundleNamed(runDir, "bundle")
}

func assessBundleNamed(runDir, subject string) (bundleAssessment, error) {
	if runDir == "" {
		return bundleAssessment{}, nil
	}
	if !fsutil.DirExists(runDir) {
		return bundleAssessment{}, fmt.Errorf("%s directory not found: %s", subject, runDir)
	}

	assessment := bundleAssessment{
		RunDir:    runDir,
		Mode:      bundleModeLegacy,
		Integrity: bundleIntegrityWarn,
	}

	manifest, ok, err := readBundleManifest(runDir)
	if err != nil {
		assessment.Integrity = bundleIntegrityBad
		assessment.Hint = fmt.Sprintf("Hint: %s %s has an unreadable manifest: %v.", subject, filepath.Base(runDir), err)
		return assessment, nil
	}
	if !ok {
		missing := missingBundleArtifacts(runDir, bundleModeExecuted)
		if len(missing) > 0 {
			assessment.Integrity = bundleIntegrityWarn
			assessment.Hint = fmt.Sprintf("Hint: %s %s is incomplete; missing %s.", subject, filepath.Base(runDir), strings.Join(missing, " and "))
		}
		return assessment, nil
	}

	assessment.Mode = manifest.Mode
	switch manifest.Mode {
	case bundleModeDryRun:
		missing := missingBundleArtifacts(runDir, bundleModeDryRun)
		if len(missing) > 0 {
			assessment.Integrity = bundleIntegrityBad
			assessment.Hint = fmt.Sprintf("Hint: %s %s is missing dry-run prompt artifact(s): %s.", subject, filepath.Base(runDir), strings.Join(missing, " and "))
			return assessment, nil
		}
	case bundleModeExecuted, bundleModeLegacy:
		missing := missingBundleArtifacts(runDir, bundleModeExecuted)
		if len(missing) > 0 {
			assessment.Integrity = bundleIntegrityWarn
			assessment.Hint = fmt.Sprintf("Hint: %s %s is incomplete; missing %s.", subject, filepath.Base(runDir), strings.Join(missing, " and "))
			return assessment, nil
		}
	default:
		missing := missingBundleArtifacts(runDir, bundleModeExecuted)
		if len(missing) > 0 {
			assessment.Integrity = bundleIntegrityWarn
			assessment.Hint = fmt.Sprintf("Hint: %s %s is incomplete; missing %s.", subject, filepath.Base(runDir), strings.Join(missing, " and "))
			return assessment, nil
		}
	}

	if err := verifyBundleManifest(runDir, manifest); err != nil {
		assessment.Integrity = bundleIntegrityBad
		assessment.Hint = fmt.Sprintf("Hint: %s %s failed integrity check: %v.", subject, filepath.Base(runDir), err)
		return assessment, nil
	}

	assessment.Integrity = bundleIntegrityOK
	assessment.ManifestOK = true
	return assessment, nil
}

func verifyBundleManifest(runDir string, manifest bundleManifest) error {
	seen := make(map[string]bundleArtifact, len(manifest.Artifacts))
	for _, artifact := range manifest.Artifacts {
		seen[artifact.Name] = artifact
	}

	for _, artifact := range manifest.Artifacts {
		path := filepath.Join(runDir, artifact.Name)
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("%s missing", artifact.Name)
		}
		if int64(len(data)) != artifact.Size {
			return fmt.Errorf("%s size changed", artifact.Name)
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != artifact.SHA256 {
			return fmt.Errorf("%s checksum changed", artifact.Name)
		}
	}

	for _, name := range expectedManifestArtifacts(manifest.Mode) {
		if _, ok := seen[name]; !ok {
			return fmt.Errorf("%s missing from manifest", name)
		}
	}

	return nil
}

func missingBundleArtifacts(runDir string, mode bundleMode) []string {
	missing := make([]string, 0, 3)
	for _, name := range expectedRunArtifacts(mode) {
		if !fsutil.FileExists(filepath.Join(runDir, name)) {
			missing = append(missing, name)
		}
	}
	return missing
}

func expectedRunArtifacts(mode bundleMode) []string {
	switch mode {
	case bundleModeDryRun:
		return []string{"prompt.txt"}
	default:
		return []string{"result.json", "diff.patch"}
	}
}

func expectedManifestArtifacts(mode bundleMode) []string {
	switch mode {
	case bundleModeDryRun:
		return []string{"prompt.txt"}
	default:
		return []string{
			"prompt.txt",
			"execution.json",
			"stdout.log",
			"stderr.log",
			"result.json",
			"diff.patch",
		}
	}
}
