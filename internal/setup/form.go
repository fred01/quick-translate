package setup

import (
	"context"
	"errors"
	"io"
	"strings"

	huh "charm.land/huh/v2"
	huhspinner "charm.land/huh/v2/spinner"

	"github.com/fred01/quick-translate/internal/app"
	"github.com/fred01/quick-translate/internal/config"
	"github.com/fred01/quick-translate/internal/translate"
)

// RunForm implements app.SetupFormRunner: it interactively names and
// configures a profile with Huh v2, then tests and saves it into the store.
// It renders to out (intended to be stderr, so stdout stays empty) and reads
// from in.
func RunForm(
	in io.Reader,
	out io.Writer,
	configPath string,
	store config.Store,
	newTranslator func(config.Config) (translate.Translator, error),
) (app.SetupResult, error) {
	name := store.Active
	if name == "" {
		name = "default"
	}
	existing := store.Profiles[name]

	candidate := existing
	var apiKeyInput string

	nameField := huh.NewInput().
		Title("Profile name").
		Description("Name of the profile to create or update (e.g. litellm, nvidia).").
		Value(&name).
		Validate(validateProfileName)

	baseURLField := huh.NewInput().
		Title("Base URL").
		Description("OpenAI-compatible API root. Must include the /v1 path, e.g. http://localhost:4000/v1").
		Value(&candidate.BaseURL).
		Validate(func(s string) error {
			_, err := config.NormalizeBaseURL(s)
			return err
		})

	modelField := huh.NewInput().
		Title("Model").
		Value(&candidate.Model).
		Validate(validateModel)

	apiKeyField := huh.NewInput().
		Title("API Key").
		Description("Leave blank to keep the existing key when editing a profile.").
		EchoMode(huh.EchoModePassword).
		Value(&apiKeyInput).
		Validate(func(s string) error {
			return validateAPIKeyInput(s, store.Profiles[name].APIKey)
		})

	form := huh.NewForm(huh.NewGroup(nameField, baseURLField, modelField, apiKeyField)).
		WithTheme(huh.ThemeFunc(huh.ThemeBase)).
		WithInput(in).
		WithOutput(out)

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return app.SetupResult{}, app.ErrInterrupted
		}
		return app.SetupResult{}, err
	}

	candidate.APIKey = resolveAPIKey(apiKeyInput, store.Profiles[name].APIKey)

	var result Result
	var testErr error
	spin := huhspinner.New().
		Title("Testing endpoint…").
		WithInput(in).
		WithOutput(out).
		ActionWithErr(func(ctx context.Context) error {
			result, testErr = TestAndSave(ctx, configPath, name, candidate, newTranslator, translate.DiscardReporter{})
			return testErr
		})
	if err := spin.Run(); err != nil {
		return app.SetupResult{}, err
	}

	return app.SetupResult{Profile: result.Name, Config: result.Config, Translation: result.Translation}, nil
}

// resolveAPIKey returns input, unless input is blank, in which case it
// returns existingKey so a blank submission keeps an existing key.
func resolveAPIKey(input, existingKey string) string {
	if strings.TrimSpace(input) == "" {
		return existingKey
	}
	return input
}

// validateProfileName rejects a blank profile name.
func validateProfileName(s string) error {
	if strings.TrimSpace(s) == "" {
		return errors.New("profile name must not be empty")
	}
	return nil
}

// validateModel rejects a blank model name.
func validateModel(s string) error {
	if strings.TrimSpace(s) == "" {
		return errors.New("model must not be empty")
	}
	return nil
}

// validateAPIKeyInput rejects a blank key input only when there is no
// existing key for it to fall back to.
func validateAPIKeyInput(input, existingKey string) error {
	if strings.TrimSpace(input) == "" && strings.TrimSpace(existingKey) == "" {
		return errors.New("API key is required")
	}
	return nil
}
