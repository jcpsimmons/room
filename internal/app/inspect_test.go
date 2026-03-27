package app

import (
	"context"
	"strings"
	"testing"

	"github.com/jcpsimmons/room/internal/agent"
	"github.com/jcpsimmons/room/internal/version"
)

func TestInspectSurfacesBundleFaultSignal(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, paths := prepareInitializedRepo(t, repoRoot)

	writeTailBundle(t, paths.RunsDir, "0001", "older prompt", &agent.Result{
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

	svc := NewService(Dependencies{
		Git:     gitClientForTailTest{},
		Version: version.Info{Version: "dev"},
	})

	report, err := svc.Inspect(context.Background(), InspectOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}

	for _, want := range []string{
		"Artifact fault signal:",
		"Hint: newest bundle 0002 is incomplete; missing result.json and diff.patch.",
	} {
		if !strings.Contains(report.Prompt, want) {
			t.Fatalf("inspect prompt missing %q:\n%s", want, report.Prompt)
		}
	}
}
