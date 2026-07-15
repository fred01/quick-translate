// Package setup implements qt setup: building a candidate profile
// configuration, testing it against the real translation path, and saving it
// into the profile store only on success.
package setup

import (
	"context"

	"github.com/fred01/quick-translate/internal/config"
	"github.com/fred01/quick-translate/internal/translate"
)

// testSource is sent, with no reference context, to verify a candidate
// configuration through the exact production Chat Completions path.
const testSource = "Привет!"

// Result is a successfully tested and saved profile, together with the
// model's test translation for display.
type Result struct {
	Name        string
	Config      config.Config
	Translation string
}

// TestAndSave resolves candidate, tests it by running the real production
// translation path against testSource, and — only if that succeeds —
// upserts it into the store at path under profileName, marks it active, and
// saves atomically. Existing profiles are preserved. It never creates or
// overwrites config on a failed test.
func TestAndSave(
	ctx context.Context,
	path string,
	profileName string,
	candidate config.Config,
	newTranslator func(config.Config) (translate.Translator, error),
	reporter translate.Reporter,
) (Result, error) {
	resolved, err := candidate.Resolve()
	if err != nil {
		return Result{}, err
	}

	translator, err := newTranslator(resolved)
	if err != nil {
		return Result{}, err
	}

	translation, err := translator.Translate(ctx, translate.TranslationInput{Source: testSource}, reporter)
	if err != nil {
		return Result{}, err
	}

	store, err := config.LoadStore(path)
	if err != nil {
		return Result{}, err
	}
	store.Upsert(profileName, resolved)
	store.Active = profileName
	if err := config.SaveStore(path, store); err != nil {
		return Result{}, err
	}

	return Result{Name: profileName, Config: resolved, Translation: translation}, nil
}
