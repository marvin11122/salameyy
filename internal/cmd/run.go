package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"

	"charm.land/log/v2"
	"github.com/spf13/cobra"
	"github.com/wiesnermarven-hue/salameyy/internal/brand"
	"github.com/wiesnermarven-hue/salameyy/internal/event"
)

var runCmd = &cobra.Command{
	Use:   "run [prompt...]",
	Short: "Run a single non-interactive prompt",
	Long: `Run a single prompt in non-interactive mode and exit.
The prompt can be provided as arguments or piped from stdin.`,
	Example: `
# Run a simple prompt
salamey run "Map the repo and fix the weakest path"

# Pipe input from stdin
curl https://charm.land | salamey run "Extract the structural moves"

# Read from a file
salamey run "What is this code doing?" <<< prrr.go

# Redirect output to a file
salamey run "Generate a sharper README for this project" > FORGED_README.md

# Run in quiet mode (hide the spinner)
salamey run --quiet "Generate a README for this project"

# Run in verbose mode (show logs)
salamey run --verbose "Generate a README for this project"
  `,
	RunE: func(cmd *cobra.Command, args []string) error {
		quiet, _ := cmd.Flags().GetBool("quiet")
		verbose, _ := cmd.Flags().GetBool("verbose")
		largeModel, _ := cmd.Flags().GetString("model")
		smallModel, _ := cmd.Flags().GetString("small-model")

		// Cancel on SIGINT or SIGTERM.
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
		defer cancel()

		app, err := setupApp(cmd)
		if err != nil {
			return err
		}
		defer app.Shutdown()

		if !app.Config().IsConfigured() {
			return fmt.Errorf("no providers configured - please run '%s' to set up a provider interactively", brand.CommandName)
		}

		if verbose {
			slog.SetDefault(slog.New(log.New(os.Stderr)))
		}

		prompt := strings.Join(args, " ")

		prompt, err = MaybePrependStdin(prompt)
		if err != nil {
			slog.Error("Failed to read from stdin", "error", err)
			return err
		}

		if prompt == "" {
			return fmt.Errorf("no prompt provided")
		}

		event.SetNonInteractive(true)
		event.AppInitialized()

		return app.RunNonInteractive(ctx, os.Stdout, prompt, largeModel, smallModel, quiet || verbose)
	},
}

func init() {
	runCmd.Flags().BoolP("quiet", "q", false, "Hide spinner")
	runCmd.Flags().BoolP("verbose", "v", false, "Show logs")
	runCmd.Flags().StringP("model", "m", "", "Model to use. Accepts 'model' or 'provider/model' to disambiguate models with the same name across providers")
	runCmd.Flags().String("small-model", "", "Small model to use. If not provided, uses the default small model for the provider")
}
