// Package commands defines the CLI surface for the ICA reference client.
package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"lumera-ica-client/client"
)

// app bundles CLI-level options and helpers shared across commands.
type app struct {
	configPath string
}

const defaultCommandTimeout = 10 * time.Minute

// NewRootCmd builds the root CLI command and registers subcommands.
func NewRootCmd() *cobra.Command {
	app := &app{}
	cmd := &cobra.Command{
		Use:          "lumera-ica-client",
		Short:        "Lumera ICA reference client",
		SilenceUsage: true,
	}
	cmd.PersistentFlags().StringVar(&app.configPath, "config", "config.toml", "Path to config file")
	cmd.AddCommand(newUploadCmd(app))
	cmd.AddCommand(newDownloadCmd(app))
	cmd.AddCommand(newActionCmd(app))
	return cmd
}

// loadConfig resolves the config path and loads the TOML config on demand.
func (a *app) loadConfig() (*client.Config, error) {
	path := strings.TrimSpace(a.configPath)
	if path == "" {
		return nil, errors.New("config path is required")
	}
	path = filepath.Clean(path)
	cfg, err := client.LoadConfig(path)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// commandContext enforces a default timeout for command execution.
func commandContext(cmd *cobra.Command) (context.Context, context.CancelFunc) {
	ctx := cmd.Context()
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, defaultCommandTimeout)
}

// writeJSON emits a pretty-printed JSON response to stdout.
func writeJSON(payload any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

// resolveOptionalArg accepts either a flag or positional value for a field.
func resolveOptionalArg(flagValue string, args []string, name string) (string, error) {
	flagValue = strings.TrimSpace(flagValue)
	if flagValue != "" {
		if len(args) > 0 {
			return "", fmt.Errorf("%s provided both as flag and argument", name)
		}
		return flagValue, nil
	}
	if len(args) > 0 {
		return strings.TrimSpace(args[0]), nil
	}
	return "", fmt.Errorf("%s is required", name)
}
