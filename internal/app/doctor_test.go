package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jcpsimmons/room/internal/agent"
	"github.com/jcpsimmons/room/internal/state"
	"github.com/jcpsimmons/room/internal/version"
)

func TestDoctorSurfacesBundleRecoveryAndDrift(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, paths := prepareInitializedRepo(t, repoRoot)

	writeTailBundle(t, paths.RunsDir, "0001", "old prompt", &agent.Result{
		Summary:         "Closed the loop",
		NextInstruction: "Listen for drift",
		Status:          "continue",
		CommitMessage:   "close the loop",
	}, strings.TrimSpace(`
diff --git a/a.txt b/a.txt
--- a/a.txt
+++ b/a.txt
@@ -1 +1,2 @@
-old
+old
+new
`))
	writeTailBundle(t, paths.RunsDir, "0002", "newest prompt", nil, "")

	snapshot, err := state.Load(paths.StatePath)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	snapshot.LastRunDirectory = filepath.Join(paths.RunsDir, "0001")
	if err := state.Save(paths.StatePath, snapshot); err != nil {
		t.Fatalf("save state: %v", err)
	}

	svc := NewService(Dependencies{
		Git:       &fakeGit{root: repoRoot},
		Providers: testProviders(&fakeRunner{version: "codex-cli 0.116.0"}, nil),
		Version:   version.Info{Version: "dev"},
	})

	report, err := svc.Doctor(context.Background(), DoctorOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}

	joined := strings.Join(report.Lines, "\n")
	for _, want := range []string{
		"Hint: newest bundle 0002 is incomplete; missing result.json and diff.patch.",
		"state points at " + filepath.Join(paths.RunsDir, "0001"),
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, joined)
		}
	}

	var bundleCheck, driftCheck bool
	for _, check := range report.Checks {
		switch check.Name {
		case "bundle":
			bundleCheck = strings.Contains(check.Message, "Hint: newest bundle 0002 is incomplete; missing result.json and diff.patch.")
		case "run_directory":
			driftCheck = strings.Contains(check.Message, "state points at") && strings.Contains(check.Message, "newest bundle")
		}
	}
	if !bundleCheck {
		t.Fatalf("expected bundle recovery check, got %#v", report.Checks)
	}
	if !driftCheck {
		t.Fatalf("expected run-directory drift check, got %#v", report.Checks)
	}
}
