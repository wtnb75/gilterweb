package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

var (
	Version = "dev"
	Commit  = "none"
	Built   = "unknown"
)

func init() {
	cobra.MousetrapHelpText = ""
	cobra.EnableCommandSorting = false
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		var ex ExitError
		if ok := AsExitError(err, &ex); ok {
			fmt.Fprintln(os.Stderr, ex.Error())
			os.Exit(ex.Code)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var configPath string
	var logLevel string

	root := &cobra.Command{
		Use:   "gilterweb",
		Short: "Filter-driven HTTP server",
	}
	root.PersistentFlags().StringVar(&configPath, "config", "config.yaml", "Config file path")
	root.PersistentFlags().StringVar(&logLevel, "log-level", "", "Override log level (debug|info|warn|error)")

	root.AddCommand(newServerCmd(&configPath, &logLevel))
	root.AddCommand(newCheckCmd(&configPath, &logLevel))
	root.AddCommand(newValidateCmd(&configPath, &logLevel))
	root.AddCommand(newVersionCmd())

	return root
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version info",
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Printf("gilterweb version %s (commit: %s, built: %s)\n", Version, Commit, Built)
		},
	}
}

func newValidateCmd(configPath *string, logLevel *string) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate config",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := LoadConfig(*configPath)
			if err != nil {
				return err
			}
			if err := applyLogLevelOverride(&cfg, *logLevel); err != nil {
				return err
			}
			if err := cfg.Validate(); err != nil {
				return err
			}
			fmt.Println("validation succeeded")
			return nil
		},
	}
}

func newServerCmd(configPath *string, logLevel *string) *cobra.Command {
	var addrOverride string
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Start HTTP server",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := LoadConfig(*configPath)
			if err != nil {
				return err
			}
			if err := applyLogLevelOverride(&cfg, *logLevel); err != nil {
				return err
			}
			if addrOverride != "" {
				cfg.Server.Addr = addrOverride
			}
			app, err := NewApp(cfg)
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			errCh := make(chan error, 1)
			go func() {
				errCh <- app.Run(ctx)
			}()

			select {
			case <-ctx.Done():
				shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
				defer cancel()
				return app.Shutdown(shutdownCtx)
			case err := <-errCh:
				return err
			}
		},
	}
	cmd.Flags().StringVar(&addrOverride, "addr", "", "Override server.addr from config")
	return cmd
}

func newCheckCmd(configPath *string, logLevel *string) *cobra.Command {
	var req CheckRequest
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Evaluate one request",
		RunE: func(_ *cobra.Command, _ []string) error {
			if req.Path == "" {
				return fmt.Errorf("--path is required")
			}
			cfg, err := LoadConfig(*configPath)
			if err != nil {
				return ExitError{Code: 1, Err: err}
			}
			if err := applyLogLevelOverride(&cfg, *logLevel); err != nil {
				return ExitError{Code: 1, Err: err}
			}
			app, err := NewApp(cfg)
			if err != nil {
				return ExitError{Code: 1, Err: err}
			}
			out, err := app.Check(context.Background(), req)
			if err != nil {
				return ExitError{Code: 2, Err: err}
			}
			fmt.Println(RenderResult(out))
			return nil
		},
	}
	cmd.Flags().StringVar(&req.Method, "method", "GET", "Request method")
	cmd.Flags().StringVar(&req.Path, "path", "", "Request path")
	cmd.Flags().StringArrayVar(&req.Headers, "header", nil, "Request header Key: Value")
	cmd.Flags().StringVar(&req.ContentType, "content-type", "", "Request Content-Type")
	cmd.Flags().StringVar(&req.Body, "body", "", "Request body")
	cmd.Flags().StringVar(&req.BodyFile, "body-file", "", "Request body file")
	return cmd
}

type ExitError struct {
	Code int
	Err  error
}

func (e ExitError) Error() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e ExitError) Unwrap() error {
	return e.Err
}

func AsExitError(err error, out *ExitError) bool {
	if err == nil {
		return false
	}
	e, ok := err.(ExitError)
	if !ok {
		return false
	}
	*out = e
	return true
}

func applyLogLevelOverride(cfg *Config, override string) error {
	if override == "" {
		return nil
	}
	if !inSet(override, "debug", "info", "warn", "error") {
		return fmt.Errorf("invalid --log-level: %s (use debug|info|warn|error)", override)
	}
	cfg.Log.Level = override
	return nil
}
