package prompt

// BuildReport captures how much context made it into the final prompt.
type BuildReport struct {
	TotalRunes                int  `json:"total_runes"`
	CurrentInstructionRunes   int  `json:"current_instruction_runes"`
	CurrentInstructionClipped bool `json:"current_instruction_clipped"`
	RecoveryHintRunes         int  `json:"recovery_hint_runes"`
	RecoveryHintClipped       bool `json:"recovery_hint_clipped"`
	RecentSummariesCount      int  `json:"recent_summaries_count"`
	PriorInstructionsCount    int  `json:"prior_instructions_count"`
	RecentCommitsCount        int  `json:"recent_commits_count"`
	GitStatusLines            int  `json:"git_status_lines"`
	GitStatusClipped          bool `json:"git_status_clipped"`
	GitStatusOmittedLines     int  `json:"git_status_omitted_lines"`
}
