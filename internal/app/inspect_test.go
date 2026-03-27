package app

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jcpsimmons/room/internal/agent"
	"github.com/jcpsimmons/room/internal/git"
	"github.com/jcpsimmons/room/internal/logs"
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

func TestInspectSurvivesMissingInstructionFile(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, paths := prepareInitializedRepo(t, repoRoot)

	if err := os.Remove(paths.InstructionPath); err != nil {
		t.Fatalf("remove instruction: %v", err)
	}
	writeTailBundle(t, paths.RunsDir, "0001", "newest prompt", nil, "")
	if err := logs.AppendSummary(paths.SummariesPath, logs.SummaryEntry{
		Iteration: 1,
		Timestamp: time.Date(2026, 3, 25, 12, 30, 0, 0, time.UTC),
		Status:    "continue",
		Summary:   "Kept the loop alive",
	}); err != nil {
		t.Fatalf("append summary: %v", err)
	}
	if err := logs.AppendSeenInstruction(paths.SeenInstructionsPath, "Hold the drift"); err != nil {
		t.Fatalf("append seen instruction: %v", err)
	}

	svc := NewService(Dependencies{
		Git: &fakeGit{
			root:        repoRoot,
			statusShort: " M a.txt",
			recentCommits: []git.Commit{{
				Hash:    "abc123",
				Subject: "close the loop",
			}},
		},
		Version: version.Info{Version: "dev"},
	})

	report, err := svc.Inspect(context.Background(), InspectOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}

	for _, want := range []string{
		"Current instruction unavailable: missing instruction.txt.",
		"Artifact fault signal:",
		"Hint: newest bundle 0001 is incomplete; missing result.json and diff.patch.",
	} {
		if !strings.Contains(report.Prompt, want) {
			t.Fatalf("inspect prompt missing %q:\n%s", want, report.Prompt)
		}
	}
	if report.RepoRoot != repoRoot {
		t.Fatalf("repo root = %q", report.RepoRoot)
	}
	if report.CurrentInstruction != "" {
		t.Fatalf("current instruction = %q", report.CurrentInstruction)
	}
	if !strings.Contains(report.RecoveryHint, "missing instruction.txt") {
		t.Fatalf("recovery hint = %q", report.RecoveryHint)
	}
	if len(report.RecentSummaries) != 1 {
		t.Fatalf("recent summaries = %#v", report.RecentSummaries)
	}
	if len(report.PriorInstructions) != 1 || report.PriorInstructions[0] != "Hold the drift" {
		t.Fatalf("prior instructions = %#v", report.PriorInstructions)
	}
	if len(report.RecentCommits) != 1 || report.RecentCommits[0].Hash != "abc123" {
		t.Fatalf("recent commits = %#v", report.RecentCommits)
	}
	if report.GitStatus != " M a.txt" {
		t.Fatalf("git status = %q", report.GitStatus)
	}
}

func TestInspectReportsPromptStats(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, paths := prepareInitializedRepo(t, repoRoot)

	longInstruction := strings.Repeat("patch the oscillator path ", 80)
	if err := os.WriteFile(paths.InstructionPath, []byte(longInstruction+"\n"), 0o644); err != nil {
		t.Fatalf("write instruction: %v", err)
	}
	if err := logs.AppendSeenInstruction(paths.SeenInstructionsPath, "Hold the drift"); err != nil {
		t.Fatalf("append seen instruction: %v", err)
	}

	svc := NewService(Dependencies{
		Git: &fakeGit{
			root:          repoRoot,
			statusShort:   strings.Join([]string{" M a.txt", "?? b.txt", "?? c.txt", "?? d.txt", "?? e.txt", "?? f.txt", "?? g.txt", "?? h.txt", "?? i.txt", "?? j.txt", "?? k.txt", "?? l.txt", "?? m.txt"}, "\n"),
			recentCommits: []git.Commit{{Hash: "abc123", Subject: "close the loop"}},
		},
		Version: version.Info{Version: "dev"},
	})

	report, err := svc.Inspect(context.Background(), InspectOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}

	if report.PromptStats.TotalRunes == 0 {
		t.Fatal("expected total runes to be recorded")
	}
	if !report.PromptStats.CurrentInstructionClipped {
		t.Fatal("expected current instruction to be marked clipped")
	}
	if !report.PromptStats.GitStatusClipped {
		t.Fatal("expected git status to be marked clipped")
	}
	if report.PromptStats.GitStatusOmittedLines != 1 {
		t.Fatalf("git status omitted lines = %d", report.PromptStats.GitStatusOmittedLines)
	}
	if report.PromptStats.RecentCommitsCount != 1 {
		t.Fatalf("recent commits count = %d", report.PromptStats.RecentCommitsCount)
	}
}

func TestInspectRedactsSensitivePromptContext(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, paths := prepareInitializedRepo(t, repoRoot)

	secretInstruction := strings.TrimSpace(`
export OPENAI_API_KEY=sk-test-1234567890abcdef1234567890
AWS_SECRET_ACCESS_KEY=abcdabcdabcdabcdabcdabcdabcdabcdabcdabcd
github_pat_abcdefghijklmnopqrstuvwxyz0123456789
-----BEGIN PRIVATE KEY-----
MIIEvQIBADANBgkqhkiG9w0BAQEFAASC
-----END PRIVATE KEY-----
`)
	if err := os.WriteFile(paths.InstructionPath, []byte(secretInstruction+"\n"), 0o644); err != nil {
		t.Fatalf("write instruction: %v", err)
	}

	svc := NewService(Dependencies{
		Git: &fakeGit{
			root:        repoRoot,
			statusShort: " M a.txt",
			recentCommits: []git.Commit{{
				Hash:    "abc123",
				Subject: "Stop leaking sk-test-1234567890abcdef1234567890",
			}},
		},
		Version: version.Info{Version: "dev"},
	})

	report, err := svc.Inspect(context.Background(), InspectOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}

	for _, want := range []string{
		"<redacted>",
		"<redacted private key>",
	} {
		if !strings.Contains(report.Prompt, want) {
			t.Fatalf("inspect prompt missing %q:\n%s", want, report.Prompt)
		}
	}
	for _, want := range []string{
		"sk-test-1234567890abcdef1234567890",
		"abcdabcdabcdabcdabcdabcdabcdabcdabcdabcd",
		"github_pat_abcdefghijklmnopqrstuvwxyz0123456789",
		"MIIEvQIBADANBgkqhkiG9w0BAQEFAASC",
	} {
		if strings.Contains(report.Prompt, want) {
			t.Fatalf("inspect prompt leaked %q:\n%s", want, report.Prompt)
		}
	}
}
