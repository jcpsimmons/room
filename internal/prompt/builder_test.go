package prompt

import (
	"strings"
	"testing"
	"time"

	"github.com/jcpsimmons/room/internal/git"
	"github.com/jcpsimmons/room/internal/logs"
)

func TestBuildIncludesRelevantContext(t *testing.T) {
	t.Parallel()

	body := Build(BuildInput{
		CurrentInstruction: "Harden the config loader",
		RecoveryHint:       "Hint: newest bundle 0002 is incomplete; missing result.json and diff.patch.",
		RecentSummaries: []logs.SummaryEntry{{
			Iteration: 3,
			Timestamp: time.Now(),
			Status:    "continue",
			Summary:   "Added config parsing tests",
		}},
		PriorInstructions: []string{"Improve config validation"},
		RecentCommits: []git.Commit{{
			Hash:    "abc123",
			Subject: "room: tighten config errors",
		}},
		GitStatus: " M internal/config/config.go",
		RepoPath:  "/tmp/repo",
	})

	for _, want := range []string{
		"Harden the config loader",
		"Added config parsing tests",
		"Improve config validation",
		"room: tighten config errors",
		"Artifact fault signal",
		"missing result.json and diff.patch",
		"M internal/config/config.go",
		"No repeated patch notes",
		"fully patched and humming",
		"Return only JSON",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("prompt missing %q", want)
		}
	}
}

func TestBuildCompactsLongContext(t *testing.T) {
	t.Parallel()

	longInstruction := strings.Repeat("patch the oscillator path ", 80)
	longSummary := strings.Repeat("phase drift ", 40)
	longCommit := strings.Repeat("rebuild the signal path ", 30)
	gitStatus := strings.Join([]string{
		" M internal/prompt/builder.go",
		" M internal/prompt/builder_test.go",
		"?? internal/prompt/extra-01.go",
		"?? internal/prompt/extra-02.go",
		"?? internal/prompt/extra-03.go",
		"?? internal/prompt/extra-04.go",
		"?? internal/prompt/extra-05.go",
		"?? internal/prompt/extra-06.go",
		"?? internal/prompt/extra-07.go",
		"?? internal/prompt/extra-08.go",
		"?? internal/prompt/extra-09.go",
		"?? internal/prompt/extra-10.go",
		"?? internal/prompt/extra-11.go",
		"?? internal/prompt/extra-12.go",
		"?? internal/prompt/extra-13.go",
	}, "\n")

	body := Build(BuildInput{
		CurrentInstruction: longInstruction,
		RecoveryHint:       strings.Repeat("bundle drift ", 40),
		RecentSummaries: []logs.SummaryEntry{{
			Iteration: 9,
			Summary:   longSummary,
		}},
		PriorInstructions: []string{strings.Repeat("reroute the filter bank ", 40)},
		RecentCommits:     []git.Commit{{Hash: "deadbeef", Subject: longCommit}},
		GitStatus:         gitStatus,
		RepoPath:          "/tmp/repo",
	})

	if !strings.Contains(body, "...") {
		t.Fatalf("expected long prompt context to be compacted")
	}
	if strings.Contains(body, "?? internal/prompt/extra-13.go") {
		t.Fatalf("expected git status to omit overflow lines")
	}
	if !strings.Contains(body, "... (+3 more lines)") {
		t.Fatalf("expected git status truncation note")
	}
	if strings.Contains(body, longInstruction) {
		t.Fatalf("expected instruction to be shortened")
	}
	if strings.Contains(body, longCommit) {
		t.Fatalf("expected commit subject to be shortened")
	}
}

func TestBuildDetailedReportsPromptSaturation(t *testing.T) {
	t.Parallel()

	body, report := BuildDetailed(BuildInput{
		CurrentInstruction: strings.Repeat("patch the oscillator path ", 80),
		RecoveryHint:       strings.Repeat("bundle drift ", 40),
		RecentSummaries: []logs.SummaryEntry{{
			Iteration: 9,
			Summary:   strings.Repeat("phase drift ", 40),
		}},
		PriorInstructions: []string{strings.Repeat("reroute the filter bank ", 40)},
		RecentCommits:     []git.Commit{{Hash: "deadbeef", Subject: strings.Repeat("rebuild the signal path ", 30)}},
		GitStatus: strings.Join([]string{
			" M internal/prompt/builder.go",
			" M internal/prompt/builder_test.go",
			"?? internal/prompt/extra-01.go",
			"?? internal/prompt/extra-02.go",
			"?? internal/prompt/extra-03.go",
			"?? internal/prompt/extra-04.go",
			"?? internal/prompt/extra-05.go",
			"?? internal/prompt/extra-06.go",
			"?? internal/prompt/extra-07.go",
			"?? internal/prompt/extra-08.go",
			"?? internal/prompt/extra-09.go",
			"?? internal/prompt/extra-10.go",
			"?? internal/prompt/extra-11.go",
			"?? internal/prompt/extra-12.go",
			"?? internal/prompt/extra-13.go",
		}, "\n"),
		RepoPath: "/tmp/repo",
	})

	if report.TotalRunes == 0 {
		t.Fatal("expected prompt to record total runes")
	}
	if !report.CurrentInstructionClipped {
		t.Fatal("expected long instruction to be marked clipped")
	}
	if !report.RecoveryHintClipped {
		t.Fatal("expected long recovery hint to be marked clipped")
	}
	if !report.GitStatusClipped {
		t.Fatal("expected long git status to be marked clipped")
	}
	if report.GitStatusOmittedLines != 3 {
		t.Fatalf("git status omitted lines = %d", report.GitStatusOmittedLines)
	}
	for _, want := range []string{
		"Patch instruction:",
		"Artifact fault signal:",
		"... (+3 more lines)",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("prompt missing %q", want)
		}
	}
}

func TestBuildRedactsObviousSecrets(t *testing.T) {
	t.Parallel()

	openAIKey := "sk-" + strings.Repeat("t", 28)
	awsSecret := strings.Repeat("a", 40)
	npmToken := "npm-secret-" + strings.Repeat("7", 12)
	githubPAT := "github_pat_" + strings.Repeat("g", 36)
	githubClassicToken := "ghp_" + strings.Repeat("h", 36)
	slackToken := "xoxb-" + strings.Repeat("1", 10) + "-" + strings.Repeat("2", 16)

	privateKey := strings.TrimSpace(`
-----BEGIN PRIVATE KEY-----
MIIEvQIBADANBgkqhkiG9w0BAQEFAASC
-----END PRIVATE KEY-----
`)
	body := Build(BuildInput{
		CurrentInstruction: strings.Join([]string{
			"Inspect the local credential dump",
			"export OPENAI_API_KEY=" + openAIKey,
			"AWS_SECRET_ACCESS_KEY=" + awsSecret,
			"//registry.npmjs.org/:_authToken=" + npmToken,
			githubPAT,
			privateKey,
		}, "\n"),
		RecoveryHint:      "Recovered from env bleed: PASSWORD=super-secret",
		PriorInstructions: []string{"Rotate the leaked token " + githubClassicToken},
		RecentSummaries: []logs.SummaryEntry{{
			Iteration: 1,
			Summary:   "Summary carried an access token: " + slackToken,
		}},
		RecentCommits: []git.Commit{{
			Hash:    "abc123",
			Subject: "Stop leaking " + openAIKey,
		}},
		GitStatus: strings.Join([]string{
			"?? secrets/" + githubPAT + ".txt",
			" M .env",
			"?? creds/" + slackToken + ".log",
		}, "\n"),
		RepoPath: "/tmp/repo",
	})

	for _, want := range []string{
		"<redacted>",
		"<redacted private key>",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected prompt to contain %q", want)
		}
	}
	for _, want := range []string{
		openAIKey,
		awsSecret,
		npmToken,
		githubPAT,
		"PASSWORD=super-secret",
		githubClassicToken,
		slackToken,
	} {
		if strings.Contains(body, want) {
			t.Fatalf("expected prompt to redact %q:\n%s", want, body)
		}
	}
	for _, want := range []string{
		"?? secrets/<redacted>.txt",
		"?? creds/<redacted>.log",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected prompt to keep redacted git status context %q:\n%s", want, body)
		}
	}
}
