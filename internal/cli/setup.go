package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fred01/quick-translate/internal/app"
	"github.com/fred01/quick-translate/internal/config"
	"github.com/fred01/quick-translate/internal/setup"
	"github.com/fred01/quick-translate/internal/translate"
)

type setupOptions struct {
	BaseURL string
	Model   string
	APIKey  string
}

func newSetupCommand(deps app.Dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Configure OpenAI-compatible endpoint profiles",
		Long: "Configure OpenAI-compatible endpoint profiles.\n\n" +
			"Run without arguments in a terminal for an interactive form, or use the\n" +
			"add/list/use/remove subcommands. Each profile has its own base URL, model,\n" +
			"and API key; one profile is active at a time.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetupInteractive(deps)
		},
	}
	cmd.AddCommand(newSetupAddCommand(deps))
	cmd.AddCommand(newSetupListCommand(deps))
	cmd.AddCommand(newSetupUseCommand(deps))
	cmd.AddCommand(newSetupRemoveCommand(deps))
	return cmd
}

func runSetupInteractive(deps app.Dependencies) error {
	if !deps.StdinIsTerminal() || !deps.StdoutIsTerminal() {
		return fail(deps, errors.New("interactive setup requires a terminal; use \"qt setup add <name> --base-url ... --model ... --api-key ...\""))
	}

	store, err := config.LoadStore(deps.ConfigPath)
	if err != nil {
		return fail(deps, fmt.Errorf("read existing config: %w", err))
	}

	result, err := deps.RunSetupForm(deps.Stdin, deps.Stderr, deps.ConfigPath, store, deps.NewTranslator)
	if err != nil {
		if errors.Is(err, app.ErrInterrupted) {
			return err
		}
		return fail(deps, err)
	}

	printSetupResult(deps, result)
	return nil
}

func newSetupAddCommand(deps app.Dependencies) *cobra.Command {
	opts := setupOptions{}
	cmd := &cobra.Command{
		Use:           "add <name>",
		Short:         "Add or update a profile, test it, and make it active",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetupAdd(cmd, args[0], opts, deps)
		},
	}
	cmd.Flags().StringVar(&opts.BaseURL, "base-url", "", "OpenAI-compatible API root, including /v1")
	cmd.Flags().StringVar(&opts.Model, "model", "", "model name")
	cmd.Flags().StringVar(&opts.APIKey, "api-key", "", "API key")
	return cmd
}

func runSetupAdd(cmd *cobra.Command, name string, opts setupOptions, deps app.Dependencies) error {
	if strings.TrimSpace(name) == "" {
		return fail(deps, errors.New("profile name must not be empty"))
	}
	if strings.TrimSpace(opts.BaseURL) == "" || strings.TrimSpace(opts.Model) == "" || strings.TrimSpace(opts.APIKey) == "" {
		return fail(deps, errors.New("--base-url, --model, and --api-key are all required"))
	}

	candidate := config.Config{BaseURL: opts.BaseURL, Model: opts.Model, APIKey: opts.APIKey}
	reporter := setupProgressReporter{BatchReporter: translate.BatchReporter{Writer: deps.Stderr}}

	result, err := setup.TestAndSave(cmd.Context(), deps.ConfigPath, name, candidate, deps.NewTranslator, reporter)
	if err != nil {
		return fail(deps, err)
	}

	printSetupResult(deps, app.SetupResult{Profile: result.Name, Config: result.Config, Translation: result.Translation})
	return nil
}

func newSetupListCommand(deps app.Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:           "list",
		Short:         "List configured profiles",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := config.LoadStore(deps.ConfigPath)
			if err != nil {
				return fail(deps, err)
			}
			names := store.Names()
			if len(names) == 0 {
				return fail(deps, errors.New("no profiles configured (run \"qt setup\")"))
			}
			for _, name := range names {
				marker := "  "
				if name == store.Active {
					marker = "* "
				}
				p := store.Profiles[name]
				fmt.Fprintf(deps.Stdout, "%s%s\t%s @ %s\n", marker, name, p.Model, translate.SafeHost(p.BaseURL))
			}
			return nil
		},
	}
}

func newSetupUseCommand(deps app.Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:           "use <name>",
		Short:         "Set the active profile",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			store, err := config.LoadStore(deps.ConfigPath)
			if err != nil {
				return fail(deps, err)
			}
			if err := store.SetActive(name); err != nil {
				return fail(deps, err)
			}
			if err := config.SaveStore(deps.ConfigPath, store); err != nil {
				return fail(deps, err)
			}
			fmt.Fprintf(deps.Stderr, "qt: active profile is now %q\n", name)
			return nil
		},
	}
}

func newSetupRemoveCommand(deps app.Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:           "remove <name>",
		Short:         "Remove a profile",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			store, err := config.LoadStore(deps.ConfigPath)
			if err != nil {
				return fail(deps, err)
			}
			if err := store.Remove(name); err != nil {
				return fail(deps, err)
			}
			if err := config.SaveStore(deps.ConfigPath, store); err != nil {
				return fail(deps, err)
			}
			fmt.Fprintf(deps.Stderr, "qt: removed profile %q\n", name)
			return nil
		},
	}
}

// setupProgressReporter reports sending/completed progress like
// translate.BatchReporter, but never prints on failure: the caller reports
// every failure itself through fail, so a failed Translate call must not
// also print through the reporter and duplicate the line.
type setupProgressReporter struct {
	translate.BatchReporter
}

func (r setupProgressReporter) Report(s translate.Status) {
	if s.Stage == translate.StageFailed {
		return
	}
	r.BatchReporter.Report(s)
}

func printSetupResult(deps app.Dependencies, result app.SetupResult) {
	fmt.Fprintf(deps.Stderr, "qt: profile %q connected to %s using model %q\n", result.Profile, translate.SafeHost(result.Config.BaseURL), result.Config.Model)
	fmt.Fprintf(deps.Stderr, "qt: test translation: %s\n", result.Translation)
}
