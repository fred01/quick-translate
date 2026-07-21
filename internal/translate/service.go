package translate

import "github.com/fred01/quick-translate/internal/config"

// NewTranslator builds the production Translator for an effective
// configuration. It is the single place that turns configuration into a
// translation backend, so every caller — batch translation, the interactive
// TUI, and qt setup's pre-save test — goes through the exact same path.
func NewTranslator(cfg config.Config) (Translator, error) {
	return NewOpenAIClient(cfg.BaseURL, cfg.APIKey, cfg.Model, cfg.Timeout())
}
