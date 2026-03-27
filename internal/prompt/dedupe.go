package prompt

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/jcpsimmons/room/internal/git"
	"github.com/jcpsimmons/room/internal/logs"
)

type DedupeInput struct {
	NextInstruction      string
	PriorInstructions    []string
	RecentSummaries      []logs.SummaryEntry
	RecentCommits        []git.Commit
	ConsecutiveNoChange  int
	ConsecutiveTinyDiffs int
}

type DedupeResult struct {
	ShouldPivot   bool     `json:"should_pivot"`
	Reasons       []string `json:"reasons"`
	Replacement   string   `json:"replacement"`
	MatchedTarget string   `json:"matched_target"`
}

var nonWordPattern = regexp.MustCompile(`[^a-z0-9\s]+`)

func DetectStagnation(input DedupeInput) DedupeResult {
	next := normalize(input.NextInstruction)
	if next == "" {
		return DedupeResult{}
	}

	for _, previous := range input.PriorInstructions {
		if normalize(previous) == next {
			return pivot(next, "exact duplicate next instruction", previous)
		}
	}

	for _, previous := range input.PriorInstructions {
		score := similarity(next, normalize(previous))
		if score >= 0.9 {
			return pivot(next, "near-duplicate next instruction", previous)
		}
	}

	if repeatedChurn(next, input.RecentSummaries) {
		return pivot(next, "repeated docs/tests/refactor churn without enough novelty", "")
	}
	if focus := repeatedFocus(next, input.PriorInstructions, input.RecentSummaries, input.RecentCommits); len(focus) > 0 {
		return pivot(next, "repeated subsystem focus across recent runs", "", focus)
	}
	if input.ConsecutiveNoChange >= 2 {
		return pivot(next, fmt.Sprintf("%d consecutive no-change iterations", input.ConsecutiveNoChange), "")
	}
	if input.ConsecutiveTinyDiffs >= 2 {
		return pivot(next, fmt.Sprintf("%d consecutive tiny or cosmetic diffs", input.ConsecutiveTinyDiffs), "")
	}
	return DedupeResult{}
}

func BuildPivotInstruction(reason string, avoid []string) string {
	base := fmt.Sprintf("Pivot hard. The prior direction is stagnating because of %s. Choose a distinctly different improvement direction. Avoid cosmetic churn. Pick another subsystem, validation layer, reliability measure, performance angle, developer-experience improvement, diagnostics capability, accessibility issue, or tooling improvement. If conventional ideas are exhausted, choose a creative but still concrete improvement.", reason)
	if len(avoid) == 0 {
		return base
	}
	return base + " Avoid these recently saturated modules: " + strings.Join(avoid, ", ") + "."
}

func normalize(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = nonWordPattern.ReplaceAllString(value, " ")
	value = strings.Join(strings.Fields(value), " ")
	return value
}

func similarity(a, b string) float64 {
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1
	}
	aa := strings.Fields(a)
	bb := strings.Fields(b)
	setA := make(map[string]struct{}, len(aa))
	for _, token := range aa {
		setA[token] = struct{}{}
	}
	shared := 0
	setB := make(map[string]struct{}, len(bb))
	for _, token := range bb {
		setB[token] = struct{}{}
		if _, ok := setA[token]; ok {
			shared++
		}
	}
	union := len(setA)
	for token := range setB {
		if _, ok := setA[token]; !ok {
			union++
		}
	}
	if union == 0 {
		return 0
	}
	return float64(shared) / float64(union)
}

func repeatedChurn(next string, summaries []logs.SummaryEntry) bool {
	churnTerms := []string{"docs", "documentation", "test", "tests", "refactor", "cleanup", "comment"}
	if !containsAny(next, churnTerms) {
		return false
	}
	recent := 0
	for _, summary := range summaries {
		if containsAny(normalize(summary.Summary), churnTerms) {
			recent++
		}
	}
	return recent >= 2
}

func repeatedFocus(next string, prior []string, summaries []logs.SummaryEntry, commits []git.Commit) []string {
	tokens := informativeTokens(next)
	if len(tokens) == 0 {
		return nil
	}
	counts := make(map[string]int, len(tokens))
	order := make([]string, 0, len(tokens))
	for _, token := range tokens {
		order = append(order, token)
	}
	for _, item := range focusEvidence(prior, summaries, commits) {
		other := informativeTokens(normalize(item))
		if len(other) == 0 {
			continue
		}
		shared := overlappingTokens(tokens, other)
		if len(shared) < 2 {
			continue
		}
		for _, token := range shared {
			counts[token]++
		}
	}
	var saturated []string
	for _, token := range order {
		if counts[token] >= 2 {
			saturated = append(saturated, token)
		}
	}
	if len(saturated) == 0 {
		return nil
	}
	sort.SliceStable(saturated, func(i, j int) bool {
		if counts[saturated[i]] == counts[saturated[j]] {
			return saturated[i] < saturated[j]
		}
		return counts[saturated[i]] > counts[saturated[j]]
	})
	if len(saturated) > 3 {
		saturated = saturated[:3]
	}
	return saturated
}

func informativeTokens(text string) []string {
	stop := map[string]struct{}{
		"the": {}, "and": {}, "for": {}, "with": {}, "that": {}, "from": {}, "this": {}, "repo": {}, "repository": {}, "improve": {}, "improvement": {}, "more": {}, "make": {}, "better": {}, "add": {}, "update": {}, "room": {}, "tests": {}, "test": {}, "docs": {}, "documentation": {}, "refactor": {}, "cleanup": {}, "performance": {}, "reliability": {}, "tooling": {}, "developer": {}, "experience": {}, "recent": {}, "runs": {}, "across": {}, "hard": {}, "avoid": {}, "choose": {}, "distinctly": {}, "different": {}, "direction": {}, "validation": {}, "layer": {}, "measure": {}, "angle": {}, "diagnostics": {}, "capability": {}, "accessibility": {}, "issue": {}, "creative": {}, "concrete": {},
	}
	var out []string
	for _, token := range strings.Fields(text) {
		if len(token) < 4 {
			continue
		}
		if _, ok := stop[token]; ok {
			continue
		}
		if !slices.Contains(out, token) {
			out = append(out, token)
		}
	}
	return out
}

func overlap(a, b []string) int {
	return len(overlappingTokens(a, b))
}

func overlappingTokens(a, b []string) []string {
	set := make(map[string]struct{}, len(a))
	for _, token := range a {
		set[token] = struct{}{}
	}
	var shared []string
	for _, token := range b {
		if _, ok := set[token]; ok {
			shared = append(shared, token)
		}
	}
	return shared
}

func containsAny(text string, terms []string) bool {
	for _, term := range terms {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}

func focusEvidence(prior []string, summaries []logs.SummaryEntry, commits []git.Commit) []string {
	items := make([]string, 0, len(prior)+len(summaries)+len(commits))
	items = append(items, prior...)
	for _, summary := range summaries {
		items = append(items, summary.Summary)
	}
	for _, commit := range commits {
		items = append(items, commit.Subject)
	}
	return items
}

func pivot(next, reason, matched string, avoid ...[]string) DedupeResult {
	var saturated []string
	if len(avoid) > 0 {
		saturated = avoid[0]
	}
	return DedupeResult{
		ShouldPivot:   true,
		Reasons:       []string{reason},
		Replacement:   BuildPivotInstruction(reason, saturated),
		MatchedTarget: matched,
	}
}
