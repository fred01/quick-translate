// Package tui implements the interactive translation UI: a compact,
// restrained Bubble Tea v2 program with Context and Source text areas, a
// read-only Translation viewport, focusable/clickable Translate, Copy, and
// Setup buttons, a spinner, and a one-line status/help footer. The Setup
// button opens an in-place profile manager (list, switch active, add, edit,
// delete) backed by the profile store.
package tui

import (
	"context"
	"errors"
	"io"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/fred01/quick-translate/internal/app"
	"github.com/fred01/quick-translate/internal/config"
	"github.com/fred01/quick-translate/internal/prompt"
	"github.com/fred01/quick-translate/internal/translate"
)

// screen identifies which top-level view the model is showing.
type screen int

const (
	screenTranslate screen = iota
	screenSetup
)

// setupMode identifies the sub-view of the profile manager.
type setupMode int

const (
	setupList setupMode = iota
	setupEdit
)

// focusTarget identifies the focused element on the translate screen.
type focusTarget int

const (
	focusSource focusTarget = iota
	focusContext
	focusTone
	focusTranslateBtn
	focusCopyBtn
)

// profile-editor field/button indices (Tab order).
const (
	editName = iota
	editBaseURL
	editModel
	editAPIKey
	editSaveBtn
	editCancelBtn
	editTargetCount
)

// buttonID identifies a clickable region for mouse hit-testing.
type buttonID int

const (
	btnNone buttonID = iota
	// Translate screen.
	btnTranslate
	btnCopy
	btnSetup
	btnQuit
	// Tone selector (translate screen).
	btnToneLiteral
	btnToneNeutral
	btnToneDiplomatic
	// Profile-manager list mode.
	btnUse
	btnAdd
	btnEdit
	btnDelete
	btnBack
	// Profile editor.
	btnSave
	btnCancel
)

// rect is an inclusive screen-cell bounding box for a clickable region.
type rect struct {
	x0, y0, x1, y1 int
}

// programHolder carries the running *tea.Program and the latest rendered
// hit-boxes into the model after construction. tea.NewProgram copies the
// model value it is given, so these cannot live on the model value directly;
// every copy shares this pointer instead. The zone maps are written by View
// (through the pointer) after each render and read by the mouse handler on
// the next update — safe because Bubble Tea runs Update and View
// sequentially on one goroutine.
type programHolder struct {
	program *tea.Program
	zones   map[buttonID]rect
	rows    []rect // profile-list row hit-boxes, indexed like the list
}

// model is the Bubble Tea model backing the interactive UI.
type model struct {
	// Configuration and the backend it drives.
	configPath    string
	getenv        config.EnvLookup
	newTranslator func(config.Config) (translate.Translator, error)
	store         config.Store
	activeName    string
	translator    translate.Translator
	modelName     string
	host          string

	holder *programHolder

	screen screen

	// Translate screen.
	source  textarea.Model
	context textarea.Model
	result  viewport.Model
	spin    spinner.Model
	focus   focusTarget
	tone    prompt.Tone
	initCmd tea.Cmd

	width, height     int
	keyDisambiguation bool

	everSubmitted   bool
	busy            bool
	cancelled       bool
	copied          bool
	notice          string
	stage           translate.Stage
	requestStarted  time.Time
	resultText      string
	lastOutputChars int
	lastElapsed     float64
	err             error
	cancel          context.CancelFunc
	reqSeq          int

	// Profile manager.
	setupMode   setupMode
	listCursor  int
	editName    string // profile being edited; "" means a new profile
	setupInputs [4]textinput.Model
	setupFocus  int
	setupBusy   bool
	setupErr    error
	setupNotice string
	setupCancel context.CancelFunc
	setupSeq    int
}

func newModel(translator translate.Translator, modelName, host string) model {
	source := textarea.New()
	source.Placeholder = "Russian text to translate"
	source.ShowLineNumbers = false
	source.Prompt = ""
	focusCmd := source.Focus()

	ctx := textarea.New()
	ctx.Placeholder = "Optional reference context"
	ctx.ShowLineNumbers = false
	ctx.Prompt = ""

	result := viewport.New()
	// Translation text is pre-wrapped at word boundaries (see
	// setResultContent), so the viewport itself must not additionally
	// hard-wrap mid-word.
	result.SoftWrap = false
	result.MouseWheelEnabled = true

	sp := spinner.New()

	nameIn := textinput.New()
	nameIn.Placeholder = "litellm"
	baseIn := textinput.New()
	baseIn.Placeholder = "http://localhost:4000/v1"
	modelIn := textinput.New()
	modelIn.Placeholder = "translategemma"
	keyIn := textinput.New()
	keyIn.EchoMode = textinput.EchoPassword
	keyIn.Placeholder = "leave blank to keep the current key"

	return model{
		translator:  translator,
		modelName:   modelName,
		host:        host,
		holder:      &programHolder{},
		screen:      screenTranslate,
		source:      source,
		context:     ctx,
		result:      result,
		spin:        sp,
		focus:       focusSource,
		tone:        prompt.DefaultTone,
		initCmd:     focusCmd,
		setupInputs: [4]textinput.Model{nameIn, baseIn, modelIn, keyIn},
	}
}

// Init implements tea.Model.
func (m model) Init() tea.Cmd {
	return m.initCmd
}

// Run launches the interactive UI. It loads the profile store from
// configPath, selects a profile (profileOverride > QT_PROFILE > active),
// builds the initial translator, and manages profiles in place. It
// implements app.TUIRunner.
func Run(
	in io.Reader,
	out io.Writer,
	configPath string,
	getenv config.EnvLookup,
	newTranslator func(config.Config) (translate.Translator, error),
	profileOverride string,
	initialTone prompt.Tone,
) error {
	store, err := config.LoadStore(configPath)
	if err != nil {
		return err
	}
	cfg, name, err := config.Effective(configPath, getenv, profileOverride)
	if err != nil {
		return err
	}
	translator, err := newTranslator(cfg)
	if err != nil {
		return err
	}

	m := newModel(translator, cfg.Model, translate.SafeHost(cfg.BaseURL))
	m.configPath = configPath
	m.getenv = getenv
	m.newTranslator = newTranslator
	m.store = store
	m.activeName = name
	m.tone = initialTone

	program := tea.NewProgram(m, tea.WithInput(in), tea.WithOutput(out))
	m.holder.program = program

	if _, err := program.Run(); err != nil {
		if errors.Is(err, tea.ErrInterrupted) {
			return app.ErrInterrupted
		}
		return err
	}
	return nil
}

// translateCmd runs one translation request in a Bubble Tea command and
// reports its outcome as a resultMsg tagged with seq.
func translateCmd(ctx context.Context, translator translate.Translator, input translate.TranslationInput, reporter translate.Reporter, seq int) tea.Cmd {
	return func() tea.Msg {
		start := time.Now()
		text, err := translator.Translate(ctx, input, reporter)
		return resultMsg{seq: seq, text: text, err: err, elapsed: time.Since(start).Seconds()}
	}
}
