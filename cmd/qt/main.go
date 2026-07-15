// Command qt translates Russian text into clear, natural, polished business
// English.
package main

import (
	"os"

	"golang.org/x/term"

	"github.com/fred01/quick-translate/internal/app"
	"github.com/fred01/quick-translate/internal/cli"
	"github.com/fred01/quick-translate/internal/config"
	"github.com/fred01/quick-translate/internal/setup"
	"github.com/fred01/quick-translate/internal/translate"
	"github.com/fred01/quick-translate/internal/tui"
)

func main() {
	configPath, err := config.Path()
	if err != nil {
		os.Stderr.WriteString("qt: " + err.Error() + "\n")
		os.Exit(1)
	}

	deps := app.Dependencies{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,

		ConfigPath: configPath,
		Getenv:     os.Getenv,

		StdinIsTerminal:  func() bool { return term.IsTerminal(int(os.Stdin.Fd())) },
		StdoutIsTerminal: func() bool { return term.IsTerminal(int(os.Stdout.Fd())) },

		NewTranslator: translate.NewTranslator,
		RunTUI:        tui.Run,
		RunSetupForm:  setup.RunForm,
	}

	root := cli.NewRootCommand(deps)
	root.SetArgs(os.Args[1:])

	os.Exit(cli.Execute(root, deps))
}
