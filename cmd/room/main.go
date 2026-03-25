package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/jcpsimmons/room/internal/app"
	"github.com/jcpsimmons/room/internal/codex"
	"github.com/jcpsimmons/room/internal/git"
	"github.com/jcpsimmons/room/internal/version"
	"github.com/spf13/cobra"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	info := version.Current()
	svc := app.NewService(app.Dependencies{
		Git:     git.NewClient(),
		Runner:  codex.NewRunner(),
		Version: info,
	})

	root := newRootCommand(ctx, svc, info)
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCommand(ctx context.Context, svc *app.Service, info version.Info) *cobra.Command {
	root := &cobra.Command{
		Use:           "room",
		Short:         "ROOM is a repo-improvement orchestrator for Codex.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(newInitCommand(ctx, svc))
	root.AddCommand(newRunCommand(ctx, svc))
	root.AddCommand(newStatusCommand(ctx, svc))
	root.AddCommand(newDoctorCommand(ctx, svc))
	root.AddCommand(newInspectCommand(ctx, svc))
	root.AddCommand(newVersionCommand(info))

	return root
}

func newInitCommand(ctx context.Context, svc *app.Service) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize ROOM state in the current repository",
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := svc.Init(ctx, app.InitOptions{WorkingDir: mustWD()})
			if err != nil {
				return err
			}
			renderLines(report.Lines)
			return nil
		},
	}
	return cmd
}

func newRunCommand(ctx context.Context, svc *app.Service) *cobra.Command {
	var opts app.RunOptions
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the improvement loop",
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts.WorkingDir = mustWD()
			opts.UntilDoneSet = cmd.Flags().Changed("until-done")
			opts.AllowDirtySet = cmd.Flags().Changed("allow-dirty")
			opts.VerboseSet = cmd.Flags().Changed("verbose")
			opts.JSONSet = cmd.Flags().Changed("json")
			report, err := svc.Run(ctx, opts)
			if opts.JSON {
				return printJSON(report, err)
			}
			if err != nil {
				return err
			}
			renderLines(report.Lines)
			return nil
		},
	}
	cmd.Flags().IntVar(&opts.Iterations, "iterations", 0, "maximum iterations to run")
	cmd.Flags().BoolVar(&opts.UntilDone, "until-done", false, "run until Codex reports done")
	cmd.Flags().IntVar(&opts.MaxFailures, "max-failures", 0, "maximum failures before stopping")
	cmd.Flags().BoolVar(&opts.NoCommit, "no-commit", false, "do not create git commits")
	cmd.Flags().BoolVar(&opts.AllowDirty, "allow-dirty", false, "allow starting from a dirty repository")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "build prompts and artifacts without executing Codex")
	cmd.Flags().BoolVar(&opts.Verbose, "verbose", false, "print more per-iteration detail")
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "emit machine-readable JSON")
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
				return printJSON(report, err)
			}
			if err != nil {
				return err
			}
			renderLines(report.Lines)
			return nil
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
				return printJSON(report, err)
			}
			if err != nil {
				return err
			}
			renderLines(report.Lines)
			return nil
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "override the config path")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit machine-readable JSON")
	return cmd
}

func newInspectCommand(ctx context.Context, svc *app.Service) *cobra.Command {
	var configPath string
	var instructionFile string
	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "Show the prompt ROOM would send to Codex",
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := svc.Inspect(ctx, app.InspectOptions{
				WorkingDir:      mustWD(),
				ConfigPath:      configPath,
				InstructionFile: instructionFile,
			})
			if err != nil {
				return err
			}
			fmt.Fprintln(os.Stdout, report.Prompt)
			return nil
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "override the config path")
	cmd.Flags().StringVar(&instructionFile, "instruction-file", "", "override the instruction file path")
	return cmd
}

func newVersionCommand(info version.Info) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show build version information",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintln(os.Stdout, info.String())
			return nil
		},
	}
}

func renderLines(lines []string) {
	for _, line := range lines {
		fmt.Fprintln(os.Stdout, line)
	}
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
	fmt.Fprintln(os.Stdout, string(data))
	return err
}

func mustWD() string {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return filepath.Clean(wd)
}
