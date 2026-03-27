package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jcpsimmons/room/internal/fsutil"
)

type bundleMode string

const (
	bundleModeDryRun      bundleMode = "dry_run"
	bundleModeExecuted    bundleMode = "executed"
	bundleModeFailed      bundleMode = "failed"
	bundleModeInterrupted bundleMode = "interrupted"
	bundleModeLegacy      bundleMode = "legacy"
	bundleIntegrityOK                = "verified"
	bundleIntegrityWarn              = "unverified"
	bundleIntegrityBad               = "mismatch"
)

var errMalformedBundleManifest = fmt.Errorf("malformed bundle manifest")

const (
	bundleIntegrityHintDecodeFailed       = "manifest_decode_failed"
	bundleIntegrityHintModeInvalid        = "manifest_mode_invalid"
	bundleIntegrityHintRunDirMissing      = "manifest_run_dir_missing"
	bundleIntegrityHintArtifactName       = "manifest_artifact_name_missing"
	bundleIntegrityHintArtifactDuplicate  = "manifest_artifact_duplicate"
	bundleIntegrityHintArtifactHash       = "manifest_artifact_hash_invalid"
	bundleIntegrityHintArtifactUnreadable = "artifact_decode_failed"
	bundleIntegrityHintFileMissing        = "artifact_file_missing"
	bundleIntegrityHintSizeChanged        = "artifact_size_changed"
	bundleIntegrityHintChecksumChanged    = "artifact_hash_changed"
	bundleIntegrityHintManifestMissing    = "manifest_artifact_missing"
	bundleIntegrityHintRunArtifactMissing = "run_artifact_missing"
)

type bundleArtifact struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type bundleManifest struct {
	RunDir    string              `json:"run_dir"`
	Mode      bundleMode          `json:"mode"`
	CreatedAt time.Time           `json:"created_at"`
	StaleLock *bundleLockRecovery `json:"stale_lock_recovery,omitempty"`
	Artifacts []bundleArtifact    `json:"artifacts"`
}

type bundleLockRecovery struct {
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
}

type BundleIntegrityHint struct {
	Code     string `json:"code"`
	Artifact string `json:"artifact,omitempty"`
	Detail   string `json:"detail"`
}

type bundleAssessment struct {
	RunDir     string
	Mode       bundleMode
	Integrity  string
	Hint       string
	Recovery   string
	ManifestOK bool
	Hints      []BundleIntegrityHint
}

func newBundleIntegrityHint(code, detail, artifact string) BundleIntegrityHint {
	return BundleIntegrityHint{
		Code:     code,
		Detail:   detail,
		Artifact: artifact,
	}
}

func writeBundleManifest(runDir string, mode bundleMode, artifactNames []string, recovery ...*bundleLockRecovery) error {
	manifest := bundleManifest{
		RunDir:    runDir,
		Mode:      mode,
		CreatedAt: time.Now().UTC(),
	}
	if len(recovery) > 0 && recovery[0] != nil {
		manifest.StaleLock = recovery[0]
	}
	for _, name := range artifactNames {
		artifactPath := filepath.Join(runDir, name)
		size, sum, err := inspectBundleArtifact(artifactPath)
		if err != nil {
			return err
		}
		manifest.Artifacts = append(manifest.Artifacts, bundleArtifact{
			Name:   name,
			Size:   size,
			SHA256: sum,
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

func readBundleManifest(runDir string) (bundleManifest, bool, []BundleIntegrityHint, error) {
	data, err := fsutil.ReadFileIfExists(filepath.Join(runDir, "bundle.json"))
	if err != nil {
		return bundleManifest{}, false, nil, err
	}
	if len(data) == 0 {
		return bundleManifest{}, false, nil, nil
	}

	var manifest bundleManifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return bundleManifest{}, false, []BundleIntegrityHint{
			newBundleIntegrityHint(bundleIntegrityHintDecodeFailed, err.Error(), ""),
		}, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			hints := []BundleIntegrityHint{
				newBundleIntegrityHint(bundleIntegrityHintDecodeFailed, "unexpected trailing data", ""),
			}
			return bundleManifest{}, false, hints, fmt.Errorf("%w: unexpected trailing data", errMalformedBundleManifest)
		}
		return bundleManifest{}, false, nil, err
	}

	hints := manifest.validate()
	if len(hints) > 0 {
		return manifest, false, hints, fmt.Errorf("%w: %v", errMalformedBundleManifest, hints[0].Detail)
	}
	return manifest, true, nil, nil
}

func (manifest bundleManifest) validate() []BundleIntegrityHint {
	hints := make([]BundleIntegrityHint, 0, 4)
	if strings.TrimSpace(manifest.RunDir) == "" {
		hints = append(hints, newBundleIntegrityHint(
			bundleIntegrityHintRunDirMissing,
			"run_dir missing",
			"",
		))
	}

	switch manifest.Mode {
	case bundleModeDryRun, bundleModeExecuted, bundleModeFailed, bundleModeInterrupted, bundleModeLegacy:
	default:
		hints = append(hints, newBundleIntegrityHint(
			bundleIntegrityHintModeInvalid,
			fmt.Sprintf("invalid mode %q", manifest.Mode),
			"mode",
		))
	}

	seen := make(map[string]struct{}, len(manifest.Artifacts))
	for _, artifact := range manifest.Artifacts {
		if strings.TrimSpace(artifact.Name) == "" {
			hints = append(hints, newBundleIntegrityHint(
				bundleIntegrityHintArtifactName,
				"artifact name missing",
				"",
			))
			continue
		}

		if _, ok := seen[artifact.Name]; ok {
			hints = append(hints, newBundleIntegrityHint(
				bundleIntegrityHintArtifactDuplicate,
				fmt.Sprintf("duplicate artifact %q", artifact.Name),
				artifact.Name,
			))
			continue
		}
		seen[artifact.Name] = struct{}{}

		if len(artifact.SHA256) != 64 {
			hints = append(hints, newBundleIntegrityHint(
				bundleIntegrityHintArtifactHash,
				fmt.Sprintf("artifact %q has malformed sha256", artifact.Name),
				artifact.Name,
			))
			continue
		}
		for _, r := range artifact.SHA256 {
			if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
				continue
			}
			hints = append(hints, newBundleIntegrityHint(
				bundleIntegrityHintArtifactHash,
				fmt.Sprintf("artifact %q has malformed sha256", artifact.Name),
				artifact.Name,
			))
			break
		}
	}

	return hints
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

	manifest, ok, manifestHints, err := readBundleManifest(runDir)
	if err != nil {
		assessment.Integrity = bundleIntegrityBad
		assessment.Hints = append(assessment.Hints, manifestHints...)
		if len(manifestHints) == 0 {
			assessment.Hints = append(assessment.Hints, newBundleIntegrityHint(
				bundleIntegrityHintDecodeFailed,
				err.Error(),
				"",
			))
		}
		assessment.Hint = fmt.Sprintf("Hint: %s %s has an unreadable manifest: %v.", subject, filepath.Base(runDir), err)
		return assessment, nil
	}
	if !ok {
		missing := missingBundleArtifacts(runDir, bundleModeExecuted)
		if len(missing) > 0 {
			assessment.Integrity = bundleIntegrityWarn
			for _, name := range missing {
				assessment.Hints = append(assessment.Hints, newBundleIntegrityHint(
					bundleIntegrityHintRunArtifactMissing,
					"required run artifact missing",
					name,
				))
			}
			assessment.Hint = fmt.Sprintf("Hint: %s %s is incomplete; missing %s.", subject, filepath.Base(runDir), strings.Join(missing, " and "))
			return assessment, nil
		}
		return assessment, nil
	}

	assessment.Mode = manifest.Mode
	switch manifest.Mode {
	case bundleModeDryRun:
		missing := missingBundleArtifacts(runDir, bundleModeDryRun)
		if len(missing) > 0 {
			assessment.Integrity = bundleIntegrityBad
			for _, name := range missing {
				assessment.Hints = append(assessment.Hints, newBundleIntegrityHint(
					bundleIntegrityHintRunArtifactMissing,
					"required run artifact missing",
					name,
				))
			}
			assessment.Hint = fmt.Sprintf("Hint: %s %s is missing dry-run prompt artifact(s): %s.", subject, filepath.Base(runDir), strings.Join(missing, " and "))
			return assessment, nil
		}
	case bundleModeExecuted, bundleModeLegacy:
		missing := missingBundleArtifacts(runDir, bundleModeExecuted)
		if len(missing) > 0 {
			assessment.Integrity = bundleIntegrityWarn
			for _, name := range missing {
				assessment.Hints = append(assessment.Hints, newBundleIntegrityHint(
					bundleIntegrityHintRunArtifactMissing,
					"required run artifact missing",
					name,
				))
			}
			assessment.Hint = fmt.Sprintf("Hint: %s %s is incomplete; missing %s.", subject, filepath.Base(runDir), strings.Join(missing, " and "))
			return assessment, nil
		}
	case bundleModeFailed, bundleModeInterrupted:
		missing := missingBundleArtifacts(runDir, manifest.Mode)
		if len(missing) > 0 {
			assessment.Integrity = bundleIntegrityWarn
			for _, name := range missing {
				assessment.Hints = append(assessment.Hints, newBundleIntegrityHint(
					bundleIntegrityHintRunArtifactMissing,
					"required run artifact missing",
					name,
				))
			}
			assessment.Hint = fmt.Sprintf("Hint: %s %s is missing failure-trace artifact(s): %s.", subject, filepath.Base(runDir), strings.Join(missing, " and "))
			return assessment, nil
		}
	default:
		missing := missingBundleArtifacts(runDir, bundleModeExecuted)
		if len(missing) > 0 {
			assessment.Integrity = bundleIntegrityWarn
			for _, name := range missing {
				assessment.Hints = append(assessment.Hints, newBundleIntegrityHint(
					bundleIntegrityHintRunArtifactMissing,
					"required run artifact missing",
					name,
				))
			}
			assessment.Hint = fmt.Sprintf("Hint: %s %s is incomplete; missing %s.", subject, filepath.Base(runDir), strings.Join(missing, " and "))
			return assessment, nil
		}
	}

	integrityHints, err := verifyBundleManifest(runDir, manifest)
	if err != nil {
		assessment.Integrity = bundleIntegrityBad
		assessment.Hints = append(assessment.Hints, integrityHints...)
		assessment.Hint = fmt.Sprintf("Hint: %s %s failed integrity check: %v.", subject, filepath.Base(runDir), err)
		return assessment, nil
	}

	assessment.Integrity = bundleIntegrityOK
	assessment.ManifestOK = true
	if manifest.StaleLock != nil {
		assessment.Recovery = fmt.Sprintf("Reclaimed stale run lock from pid %d started %s.", manifest.StaleLock.PID, manifest.StaleLock.StartedAt.UTC().Format(time.RFC3339))
	}
	return assessment, nil
}

func verifyBundleManifest(runDir string, manifest bundleManifest) ([]BundleIntegrityHint, error) {
	hints := make([]BundleIntegrityHint, 0, len(manifest.Artifacts))
	seen := make(map[string]bundleArtifact, len(manifest.Artifacts))
	for _, artifact := range manifest.Artifacts {
		seen[artifact.Name] = artifact
	}

	for _, artifact := range manifest.Artifacts {
		path := filepath.Join(runDir, artifact.Name)
		size, sum, err := inspectBundleArtifact(path)
		if err != nil {
			hints = append(hints, newBundleIntegrityHint(
				bundleIntegrityHintFileMissing,
				"artifact file missing",
				artifact.Name,
			))
			return hints, fmt.Errorf("%s missing", artifact.Name)
		}
		if size != artifact.Size {
			hints = append(hints, newBundleIntegrityHint(
				bundleIntegrityHintSizeChanged,
				"artifact size changed",
				artifact.Name,
			))
			return hints, fmt.Errorf("%s size changed", artifact.Name)
		}
		if sum != artifact.SHA256 {
			hints = append(hints, newBundleIntegrityHint(
				bundleIntegrityHintChecksumChanged,
				"artifact checksum changed",
				artifact.Name,
			))
			return hints, fmt.Errorf("%s checksum changed", artifact.Name)
		}
	}

	for _, name := range expectedManifestArtifacts(manifest.Mode) {
		if _, ok := seen[name]; !ok {
			hints = append(hints, newBundleIntegrityHint(
				bundleIntegrityHintManifestMissing,
				"artifact missing from manifest",
				name,
			))
			return hints, fmt.Errorf("%s missing from manifest", name)
		}
	}

	return nil, nil
}

func inspectBundleArtifact(path string) (int64, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer file.Close()

	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return 0, "", err
	}
	return size, hex.EncodeToString(hash.Sum(nil)), nil
}

func appendArtifactDecodeWarning(assessment *bundleAssessment, subject, artifact string, err error) {
	if assessment == nil || err == nil {
		return
	}

	assessment.Hints = append(assessment.Hints, newBundleIntegrityHint(
		bundleIntegrityHintArtifactUnreadable,
		strings.TrimSpace(err.Error()),
		artifact,
	))
	if assessment.Integrity == bundleIntegrityOK {
		assessment.Integrity = bundleIntegrityWarn
	}

	message := fmt.Sprintf("Hint: %s %s contains unreadable %s: %v.", subject, filepath.Base(assessment.RunDir), artifact, err)
	if strings.TrimSpace(assessment.Hint) == "" {
		assessment.Hint = message
		return
	}
	assessment.Hint += " " + message
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
	case bundleModeFailed, bundleModeInterrupted:
		return []string{"prompt.txt", "execution.json", "stdout.log", "stderr.log"}
	default:
		return []string{"result.json", "diff.patch"}
	}
}

func expectedManifestArtifacts(mode bundleMode) []string {
	switch mode {
	case bundleModeDryRun:
		return []string{"prompt.txt"}
	case bundleModeFailed, bundleModeInterrupted:
		return []string{"prompt.txt", "execution.json", "stdout.log", "stderr.log"}
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

func manifestHintsJSON(hints []BundleIntegrityHint) string {
	if len(hints) == 0 {
		return "[]"
	}
	data, err := json.Marshal(hints)
	if err != nil {
		return "[]"
	}
	return string(data)
}
