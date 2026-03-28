package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jcpsimmons/room/internal/agent"
	"github.com/jcpsimmons/room/internal/app"
	"github.com/jcpsimmons/room/internal/claude"
	"github.com/jcpsimmons/room/internal/git"
	"github.com/jcpsimmons/room/internal/ui"
	"github.com/jcpsimmons/room/internal/version"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	info := version.Current()
	svc := app.NewService(app.Dependencies{
		Git:     git.NewClient(),
		Version: info,
	})

	root := newRootCommand(ctx, svc, info)
	if err := root.ExecuteContext(ctx); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCommand(ctx context.Context, svc *app.Service, info version.Info) *cobra.Command {
	root := &cobra.Command{
		Use:           "room",
		Short:         "ROOM is a repo-improvement orchestrator for Codex and Claude Code.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(newInitCommand(ctx, svc))
	root.AddCommand(newRunCommand(ctx, svc))
	root.AddCommand(newStatusCommand(ctx, svc))
	root.AddCommand(newDoctorCommand(ctx, svc))
	root.AddCommand(newInspectCommand(ctx, svc))
	root.AddCommand(newConfigCommand(ctx, svc))
	root.AddCommand(newConfigCheckCommand(ctx, svc))
	root.AddCommand(newBundleCommand(ctx, svc))
	root.AddCommand(newTailCommand(ctx, svc))
	root.AddCommand(newPruneCommand(ctx, svc))
	root.AddCommand(newVersionCommand(info))

	return root
}

func newInitCommand(ctx context.Context, svc *app.Service) *cobra.Command {
	var initialPrompt string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize ROOM state in the current repository",
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolvedPrompt, err := resolveInitPrompt(initialPrompt, os.Stdin)
			if err != nil {
				return err
			}
			report, err := svc.Init(ctx, app.InitOptions{
				WorkingDir:    mustWD(),
				InitialPrompt: resolvedPrompt,
			})
			if err != nil {
				return err
			}
			return renderInit(report)
		},
	}
	cmd.Flags().StringVar(&initialPrompt, "prompt", "", "seed the initial instruction; use '-' to read from stdin")
	return cmd
}

func newRunCommand(ctx context.Context, svc *app.Service) *cobra.Command {
	var opts app.RunOptions
	var noSound bool
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the improvement loop",
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts.WorkingDir = mustWD()
			opts.UntilDoneSet = cmd.Flags().Changed("until-done")
			opts.AllowDirtySet = cmd.Flags().Changed("allow-dirty")
			opts.VerboseSet = cmd.Flags().Changed("verbose")
			opts.JSONSet = cmd.Flags().Changed("json")
			if !opts.JSON && canStyleOutput() {
				if shouldUseRunUI() {
					return runWithUI(ctx, svc, opts, noSound)
				}
				return runWithLiveProgress(ctx, svc, opts)
			}
			if !opts.JSON {
				report, err := svc.Run(ctx, opts)
				if err != nil {
					return err
				}
				return renderLines(report.Lines)
			}
			return runWithJSON(ctx, svc, opts)
		},
	}
	cmd.Flags().IntVar(&opts.Iterations, "iterations", 0, "maximum iterations to run")
	cmd.Flags().BoolVar(&opts.UntilDone, "until-done", false, "run until the selected agent reports done")
	cmd.Flags().IntVar(&opts.MaxFailures, "max-failures", 0, "maximum failures before stopping")
	cmd.Flags().BoolVar(&opts.NoCommit, "no-commit", false, "do not create git commits")
	cmd.Flags().BoolVar(&opts.AllowDirty, "allow-dirty", false, "allow starting from a dirty repository")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "build prompts and artifacts without executing the agent")
	cmd.Flags().BoolVar(&opts.Verbose, "verbose", false, "print more per-iteration detail")
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "emit newline-delimited JSON progress events and the final result")
	cmd.Flags().BoolVar(&noSound, "no-sound", false, "disable TUI sound output")
	cmd.Flags().StringVar(&opts.InstructionFile, "instruction-file", "", "override the instruction file path")
	cmd.Flags().StringVar(&opts.ConfigPath, "config", "", "override the config path")
	cmd.Flags().StringVar(&opts.CommitPrefix, "commit-prefix", "", "override the configured commit prefix")
	return cmd
}

func newStatusCommand(ctx context.Context, svc *app.Service) *cobra.Command {
	var configPath string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show current ROOM state",
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := svc.Status(ctx, app.StatusOptions{
				WorkingDir: mustWD(),
				ConfigPath: configPath,
			})
			if asJSON {
				return writeStatusJSON(os.Stdout, report, err)
			}
			if err != nil {
				return err
			}
			return renderStatus(report)
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "override the config path")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit machine-readable JSON")
	return cmd
}

func newDoctorCommand(ctx context.Context, svc *app.Service) *cobra.Command {
	var configPath string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run environment and repository checks",
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := svc.Doctor(ctx, app.DoctorOptions{
				WorkingDir: mustWD(),
				ConfigPath: configPath,
			})
			if asJSON {
				return writeDoctorJSON(os.Stdout, report, err)
			}
			if err != nil {
				return err
			}
			return renderDoctor(report)
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "override the config path")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit machine-readable JSON")
	return cmd
}

func newInspectCommand(ctx context.Context, svc *app.Service) *cobra.Command {
	var configPath string
	var instructionFile string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "Show the prompt ROOM would send to the selected agent",
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := svc.Inspect(ctx, app.InspectOptions{
				WorkingDir:      mustWD(),
				ConfigPath:      configPath,
				InstructionFile: instructionFile,
			})
			if asJSON {
				return printJSON(report, err)
			}
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(os.Stdout, report.Prompt)
			return err
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "override the config path")
	cmd.Flags().StringVar(&instructionFile, "instruction-file", "", "override the instruction file path")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit machine-readable JSON")
	return cmd
}

func newConfigCommand(ctx context.Context, svc *app.Service) *cobra.Command {
	var configPath string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show the resolved ROOM configuration",
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := svc.Config(ctx, app.ConfigOptions{
				WorkingDir: mustWD(),
				ConfigPath: configPath,
			})
			if asJSON {
				return printJSON(report, err)
			}
			if err != nil {
				return err
			}
			return renderLines(report.Lines)
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "override the config path")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit machine-readable JSON")
	return cmd
}

func newConfigCheckCommand(ctx context.Context, svc *app.Service) *cobra.Command {
	var configPath string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "config-check",
		Short: "Validate the ROOM config before starting a run",
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := svc.ConfigCheck(ctx, app.ConfigCheckOptions{
				WorkingDir: mustWD(),
				ConfigPath: configPath,
			})
			if asJSON {
				return printJSON(report, err)
			}
			if err != nil {
				return err
			}
			return renderLines(report.Lines)
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "override the config path")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit machine-readable JSON")
	return cmd
}

func newBundleCommand(ctx context.Context, svc *app.Service) *cobra.Command {
	var configPath string
	var runDir string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "bundle",
		Short: "Inspect a ROOM run bundle",
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := svc.Bundle(ctx, app.BundleOptions{
				WorkingDir: mustWD(),
				ConfigPath: configPath,
				RunDir:     runDir,
			})
			if asJSON {
				return printJSON(report, err)
			}
			if err != nil {
				return err
			}
			return renderLines(report.Lines)
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "override the config path")
	cmd.Flags().StringVar(&runDir, "run-dir", "", "inspect a specific bundle directory instead of the newest one")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit machine-readable JSON")
	return cmd
}

func newTailCommand(ctx context.Context, svc *app.Service) *cobra.Command {
	var configPath string
	cmd := &cobra.Command{
		Use:   "tail",
		Short: "Show the newest ROOM run bundle",
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := svc.Tail(ctx, app.TailOptions{
				WorkingDir: mustWD(),
				ConfigPath: configPath,
			})
			if err != nil {
				return err
			}
			return renderLines(report.Lines)
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "override the config path")
	return cmd
}

func newPruneCommand(ctx context.Context, svc *app.Service) *cobra.Command {
	var configPath string
	var keep int
	var dryRun bool
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove older ROOM run bundles",
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := svc.Prune(ctx, app.PruneOptions{
				WorkingDir: mustWD(),
				ConfigPath: configPath,
				Keep:       keep,
				DryRun:     dryRun,
			})
			if asJSON {
				return printJSON(report, err)
			}
			if err != nil {
				return err
			}
			return renderLines(report.Lines)
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "override the config path")
	cmd.Flags().IntVar(&keep, "keep", 10, "number of newest run bundles to keep")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be deleted without removing anything")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit machine-readable JSON")
	return cmd
}

func newVersionCommand(info version.Info) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show build version information",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(os.Stdout, info.String())
			return err
		},
	}
}

func runWithUI(ctx context.Context, svc *app.Service, opts app.RunOptions, noSound bool) error {
	total := opts.Iterations
	if total <= 0 {
		total = 1
	}

	model := ui.NewRunModel(total, runUIOptions(noSound)...)
	program := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithContext(ctx),
		tea.WithInput(nil),
		tea.WithoutSignalHandler(),
	)

	type runResult struct {
		report app.RunReport
		err    error
	}

	resultCh := make(chan runResult, 1)
	opts.Progress = func(event app.RunProgressEvent) {
		program.Send(ui.ProgressMsg{Event: toUIProgressEvent(event)})
	}

	go func() {
		report, err := svc.Run(ctx, opts)
		resultCh <- runResult{report: report, err: err}
		time.Sleep(120 * time.Millisecond)
		program.Quit()
	}()

	_, uiErr := program.Run()
	model.Shutdown()
	result := <-resultCh

	renderErr := renderRun(result.report)
	if uiErr != nil && !errors.Is(uiErr, tea.ErrProgramKilled) {
		return errors.Join(result.err, renderErr, uiErr)
	}
	return errors.Join(result.err, renderErr)
}

func runUIOptions(noSound bool) []ui.RunOption {
	if noSound {
		return nil
	}
	return []ui.RunOption{ui.WithAudio()}
}

func runWithLiveProgress(ctx context.Context, svc *app.Service, opts app.RunOptions) error {
	opts.Progress = func(event app.RunProgressEvent) {
		for _, line := range formatRunProgress(event) {
			_, _ = fmt.Fprintln(os.Stdout, line)
		}
	}

	report, err := svc.Run(ctx, opts)
	renderErr := renderRun(report)
	return errors.Join(err, renderErr)
}

func runWithJSON(ctx context.Context, svc *app.Service, opts app.RunOptions) error {
	stream := newJSONLineWriter(os.Stdout)
	opts.Progress = func(event app.RunProgressEvent) {
		stream.Write(makeRunJSONProgressLine(event))
	}

	report, err := svc.Run(ctx, opts)
	stream.Write(makeRunJSONResultLine(report, err))
	return errors.Join(err, stream.Err())
}

const runJSONSchemaVersion = 1

type jsonLineWriter struct {
	enc *json.Encoder
	err error
}

func newJSONLineWriter(w io.Writer) *jsonLineWriter {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return &jsonLineWriter{enc: enc}
}

func (w *jsonLineWriter) Write(v any) {
	if w.err != nil {
		return
	}
	w.err = w.enc.Encode(v)
}

func (w *jsonLineWriter) Err() error {
	return w.err
}

type runJSONProgressEvent struct {
	SchemaVersion int    `json:"schema_version"`
	Type          string `json:"type"`
	app.RunProgressEvent
	Error string `json:"error,omitempty"`
}

type runJSONResultLine struct {
	SchemaVersion int           `json:"schema_version"`
	Type          string        `json:"type"`
	OK            bool          `json:"ok"`
	Result        app.RunReport `json:"result,omitempty"`
	Error         string        `json:"error,omitempty"`
}

func makeRunJSONProgressLine(event app.RunProgressEvent) runJSONProgressEvent {
	line := runJSONProgressEvent{SchemaVersion: runJSONSchemaVersion, Type: "progress", RunProgressEvent: event}
	if event.Err != nil {
		line.Error = event.Err.Error()
	}
	return line
}

func makeRunJSONResultLine(report app.RunReport, err error) runJSONResultLine {
	return runJSONResultLine{
		SchemaVersion: runJSONSchemaVersion,
		Type:          "result",
		OK:            err == nil,
		Result:        report,
		Error:         errorText(err, ""),
	}
}

func formatRunProgress(event app.RunProgressEvent) []string {
	switch event.Phase {
	case app.RunProgressPhaseRunStart:
		return []string{
			fmt.Sprintf("ROOM run in %s", event.RepoRoot),
			fmt.Sprintf("Provider: %s", agent.DisplayName(event.Provider)),
			fmt.Sprintf("Iterations requested: %d", event.RequestedIterations),
			fmt.Sprintf("Commit mode: %t", event.CommitEnabled),
		}
	case app.RunProgressPhaseIterationStart:
		return []string{fmt.Sprintf("Starting iteration %d...", event.Iteration)}
	case app.RunProgressPhaseAgentExecutionStart:
		return []string{fmt.Sprintf("Executing iteration %d with %s...", event.Iteration, agent.DisplayName(event.Provider))}
	case app.RunProgressPhaseAgentExecutionPulse:
		return []string{fmt.Sprintf(
			"Iteration %d still running with %s after %s...",
			event.Iteration,
			agent.DisplayName(event.Provider),
			formatHeartbeatDuration(event.ExecutionElapsedMS),
		)}
	case app.RunProgressPhaseIterationSuccess:
		if event.DryRun {
			return []string{fmt.Sprintf("Dry run prepared prompt for iteration %d at %s", event.Iteration, event.PromptPath)}
		}
		detail := strings.TrimSpace(event.Summary)
		if detail == "" {
			detail = "iteration completed cleanly"
		}
		return []string{fmt.Sprintf("Iteration %d [%s]: %s", event.Iteration, event.Status, detail)}
	case app.RunProgressPhaseIterationFailure:
		failureText := errorText(event.Err, "agent execution failed")
		if exit := formatProgressExit(event); exit != "" {
			failureText = fmt.Sprintf("%s [%s]", failureText, exit)
		}
		if errors.Is(event.Err, claude.ErrMalformedOutputEnvelope) {
			return []string{fmt.Sprintf("Iteration %d failed: %s", event.Iteration, failureText)}
		}
		return []string{fmt.Sprintf("Iteration %d failed: %s", event.Iteration, failureText)}
	case app.RunProgressPhaseRunFinish:
		if event.Err != nil {
			if errors.Is(event.Err, claude.ErrMalformedOutputEnvelope) {
				return []string{fmt.Sprintf("Run halted: %s", errorText(event.Err, "Claude wrapper drift detected"))}
			}
			return []string{fmt.Sprintf("Run halted: %s", event.Err.Error())}
		}
		if event.StoppedOnDone {
			return []string{"Agent reported done. Stopping."}
		}
		return nil
	default:
		return nil
	}
}

func shouldUseRunUI() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("ROOM_TUI"))) {
	case "0", "false", "no", "off":
		return false
	case "1", "true", "yes", "on":
		return true
	default:
		return true
	}
}

func toUIProgressEvent(event app.RunProgressEvent) ui.ProgressEvent {
	total := event.RequestedIterations
	if total <= 0 {
		total = max(event.CompletedIterations+1, 1)
	}

	progressValue := 0.0
	if total > 0 {
		progressValue = float64(event.CompletedIterations) / float64(total)
	}

	out := ui.ProgressEvent{
		Kind:         ui.ProgressStep,
		Iteration:    event.Iteration,
		Total:        total,
		Completed:    event.CompletedIterations,
		Failures:     event.Failures,
		Percent:      progressValue,
		HasIteration: event.Iteration > 0,
		HasTotal:     total > 0,
		HasCompleted: true,
		HasFailures:  true,
		HasPercent:   total > 0,
		When:         progressWhen(event),
		Stdout:       event.StdoutFragment,
		Stderr:       event.StderrFragment,

		Provider:      event.Provider,
		Model:         event.Model,
		RepoRoot:      event.RepoRoot,
		CommitEnabled: event.CommitEnabled,
		DryRun:        event.DryRun,
	}

	switch event.Phase {
	case app.RunProgressPhaseRunStart:
		out.Kind = ui.ProgressStart
		out.Title = "voltage applied"
		out.Detail = fmt.Sprintf("patched %s through %s", event.RepoRoot, strings.ToUpper(event.Provider))
	case app.RunProgressPhaseIterationStart:
		out.Kind = ui.ProgressStep
		out.Title = fmt.Sprintf("step %d gate open", event.Iteration)
		out.Detail = event.RunDir
	case app.RunProgressPhaseAgentExecutionStart:
		out.Kind = ui.ProgressStep
		out.Title = fmt.Sprintf("%s oscillating", strings.ToUpper(event.Provider))
		out.Detail = fmt.Sprintf("step %d signal routed", event.Iteration)
	case app.RunProgressPhaseAgentExecutionPulse:
		out.Kind = ui.ProgressMessageKind
		out.Title = fmt.Sprintf("%s carrier wave stable", strings.ToUpper(event.Provider))
		out.Detail = fmt.Sprintf("step %d in flight for %s", event.Iteration, formatHeartbeatDuration(event.ExecutionElapsedMS))
	case app.RunProgressPhaseIterationSuccess:
		out.Kind = ui.ProgressComplete
		out.Title = fmt.Sprintf("step %d output captured", event.Iteration)
		if event.Summary != "" {
			out.Detail = event.Summary
		} else if event.DryRun {
			out.Detail = "monitor mode, no signal committed to tape"
		} else {
			out.Detail = "waveform recorded"
		}
		switch event.Status {
		case "pivot":
			out.Kind = ui.ProgressPivot
		case "done":
			out.Kind = ui.ProgressDone
			out.Percent = 1
			out.HasPercent = true
		}
	case app.RunProgressPhaseIterationFailure:
		out.Kind = ui.ProgressFailure
		out.Title = fmt.Sprintf("step %d overloaded", event.Iteration)
		out.Detail = formatProgressFailureDetail(event)
	case app.RunProgressPhaseRunFinish:
		switch {
		case event.Err != nil:
			out.Kind = ui.ProgressFailure
			out.Title = "sequence interrupted"
			out.Detail = event.Err.Error()
		case event.Status == "dry_run":
			out.Kind = ui.ProgressComplete
			out.Title = "monitor pass complete"
			out.Detail = "all signals observed, nothing committed to tape"
			out.Percent = 1
			out.HasPercent = true
		case event.Status == "done":
			out.Kind = ui.ProgressDone
			out.Title = "sequence complete"
			out.Detail = "all voices report silence"
			out.Percent = 1
			out.HasPercent = true
		default:
			out.Kind = ui.ProgressComplete
			out.Title = "sequence ended"
			out.Detail = fmt.Sprintf("%d steps through the filter", event.CompletedIterations)
			if total > 0 && event.CompletedIterations >= total {
				out.Percent = 1
				out.HasPercent = true
			}
		}
	}

	return out
}

func progressWhen(event app.RunProgressEvent) time.Time {
	switch {
	case !event.FinishedAt.IsZero():
		return event.FinishedAt
	case !event.StartedAt.IsZero():
		return event.StartedAt
	default:
		return time.Now()
	}
}

func formatHeartbeatDuration(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	return (time.Duration(ms) * time.Millisecond).Round(100 * time.Millisecond).String()
}

func formatProgressFailureDetail(event app.RunProgressEvent) string {
	detail := errorText(event.Err, "signal clipped")
	if exit := formatProgressExit(event); exit != "" {
		return fmt.Sprintf("%s [%s]", detail, exit)
	}
	return detail
}

func formatProgressExit(event app.RunProgressEvent) string {
	switch {
	case strings.TrimSpace(event.ExitSignal) != "" && event.ExitCode != 0:
		return fmt.Sprintf("exit %d via %s", event.ExitCode, strings.TrimSpace(event.ExitSignal))
	case strings.TrimSpace(event.ExitSignal) != "":
		return fmt.Sprintf("signal %s", strings.TrimSpace(event.ExitSignal))
	case event.ExitCode != 0:
		return fmt.Sprintf("exit %d", event.ExitCode)
	default:
		return ""
	}
}

func renderInit(report app.InitReport) error {
	if !canStyleOutput() {
		return renderLines(report.Lines)
	}

	summary := ui.InitSummary{
		RepoRoot:  report.RepoRoot,
		RoomDir:   report.RoomDir,
		NextSteps: []string{"room doctor", "room inspect", "room run --iterations 5"},
	}
	for _, line := range report.Lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "",
			strings.HasPrefix(trimmed, "Initialized ROOM in "),
			strings.HasPrefix(trimmed, "State directory:"),
			trimmed == "Next steps:",
			trimmed == "room doctor",
			trimmed == "room inspect",
			trimmed == "room run --iterations 5":
			continue
		}
		if strings.HasPrefix(trimmed, "ROOM ignores ") || strings.HasPrefix(trimmed, "Recommendation:") {
			summary.MissingIgnore = true
			if summary.IgnoreAdvisory != "" {
				summary.IgnoreAdvisory += "\n"
			}
			summary.IgnoreAdvisory += trimmed
			continue
		}
		summary.Notes = append(summary.Notes, trimmed)
	}
	return renderBlock(ui.RenderInit(summary))
}

func resolveInitPrompt(raw string, stdin io.Reader) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}

	prompt := raw
	if raw == "-" {
		if stdin == nil {
			return "", errors.New("stdin is not available")
		}
		data, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("read initial prompt from stdin: %w", err)
		}
		prompt = string(data)
	}

	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", errors.New("initial prompt cannot be empty")
	}
	return prompt, nil
}

func renderStatus(report app.StatusReport) error {
	if !canStyleOutput() {
		return renderLines(report.Lines)
	}

	recentSummaries := make([]string, 0, len(report.RecentSummaries))
	for _, summary := range report.RecentSummaries {
		recentSummaries = append(recentSummaries, fmt.Sprintf("#%d [%s] %s", summary.Iteration, summary.Status, summary.Summary))
	}

	return renderBlock(ui.RenderStatus(ui.StatusSummary{
		RepoRoot:           report.RepoRoot,
		Provider:           report.Provider,
		Iteration:          report.State.CurrentIteration,
		LastRun:            formatStatusTime(report.State.LastRunAt),
		LastStatus:         report.State.LastStatus,
		BundleHint:         report.LatestBundleHint,
		RecoveryHint:       report.LatestBundleRecovery,
		RoomIgnoreHint:     report.RoomIgnoreHint,
		Dirty:              report.Dirty,
		CurrentInstruction: report.CurrentInstruction,
		RecentCommits:      report.RecentCommits,
		RecentSummaries:    recentSummaries,
	}))
}

func renderDoctor(report app.DoctorReport) error {
	if !canStyleOutput() {
		return renderLines(report.Lines)
	}

	checks := make([]ui.Check, 0, len(report.Checks))
	var notes []string
	for _, check := range report.Checks {
		checks = append(checks, ui.Check{
			Name:    check.Name,
			OK:      check.OK,
			Message: check.Message,
		})
		if check.Name == "expectation" || (check.Name == "state" && strings.Contains(check.Message, "not initialized")) {
			notes = append(notes, check.Message)
		}
		if check.Name == "bundle" || check.Name == "run_directory" || check.Name == "history" || check.Name == "prompt_history" {
			notes = append(notes, check.Message)
		}
	}

	return renderBlock(ui.RenderDoctor(ui.DoctorSummary{
		RepoRoot: report.RepoRoot,
		Checks:   checks,
		Notes:    notes,
	}))
}

const doctorJSONSchemaVersion = 1

type versionedJSONResultLine[T any] struct {
	SchemaVersion int    `json:"schema_version"`
	Type          string `json:"type"`
	OK            bool   `json:"ok"`
	Result        T      `json:"result,omitempty"`
	Error         string `json:"error,omitempty"`
}

func writeDoctorJSON(w io.Writer, report app.DoctorReport, err error) error {
	return writeVersionedJSONResult(w, doctorJSONSchemaVersion, report, err)
}

func writeStatusJSON(w io.Writer, report app.StatusReport, err error) error {
	return writeVersionedJSONResult(w, doctorJSONSchemaVersion, report, err)
}

func writeVersionedJSONResult[T any](w io.Writer, schemaVersion int, report T, err error) error {
	payload := versionedJSONResultLine[T]{
		SchemaVersion: schemaVersion,
		Type:          "result",
		OK:            err == nil,
		Result:        report,
		Error:         errorText(err, ""),
	}
	data, marshalErr := json.MarshalIndent(payload, "", "  ")
	if marshalErr != nil {
		return errors.Join(err, marshalErr)
	}
	_, writeErr := fmt.Fprintln(w, string(data))
	return errors.Join(err, writeErr)
}

func renderRun(report app.RunReport) error {
	if !canStyleOutput() {
		return renderLines(report.Lines)
	}

	return renderBlock(ui.RenderRun(ui.RunSummary{
		RepoRoot:            report.RepoRoot,
		Provider:            report.Provider,
		RequestedIterations: report.RequestedIterations,
		CompletedIterations: report.CompletedIterations,
		Failures:            report.Failures,
		LastStatus:          report.LastStatus,
		LastRunDir:          report.LastRunDir,
		Timeline:            report.Lines,
	}))
}

func renderBlock(block string) error {
	_, err := fmt.Fprintln(os.Stdout, block)
	return err
}

func renderLines(lines []string) error {
	for _, line := range lines {
		if _, err := fmt.Fprintln(os.Stdout, line); err != nil {
			return err
		}
	}
	return nil
}

func printJSON(v any, err error) error {
	payload := map[string]any{"ok": err == nil}
	if v != nil {
		payload["result"] = v
	}
	if err != nil {
		payload["error"] = err.Error()
	}
	data, marshalErr := json.MarshalIndent(payload, "", "  ")
	if marshalErr != nil {
		return errors.Join(err, marshalErr)
	}
	_, writeErr := fmt.Fprintln(os.Stdout, string(data))
	return errors.Join(err, writeErr)
}

func mustWD() string {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return filepath.Clean(wd)
}

func canStyleOutput() bool {
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func formatStatusTime(value any) string {
	switch t := value.(type) {
	case interface {
		IsZero() bool
		Format(string) string
	}:
		if t.IsZero() {
			return "never"
		}
		return t.Format(time.RFC3339)
	default:
		return "unknown"
	}
}

func errorText(err error, fallback string) string {
	if err == nil {
		return fallback
	}
	return err.Error()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
