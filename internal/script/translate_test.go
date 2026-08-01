package script

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/fred01/quick-translate/internal/prompt"
	"github.com/fred01/quick-translate/internal/translate"
)

// recordingTranslator answers with whatever respond returns for a given
// request, and records every request it was given, so tests can assert on what
// was actually sent - which for this package matters as much as the output.
type recordingTranslator struct {
	respond func(call int, input translate.TranslationInput) (string, error)
	inputs  []translate.TranslationInput
}

func (r *recordingTranslator) Translate(_ context.Context, input translate.TranslationInput, reporter translate.Reporter) (string, error) {
	r.inputs = append(r.inputs, input)
	reply, err := r.respond(len(r.inputs), input)
	if err != nil {
		reporter.Report(translate.Status{Stage: translate.StageFailed, Err: err})
		return "", err
	}
	reporter.Report(translate.Status{Stage: translate.StageCompleted})
	return reply, nil
}

// echoNumbered answers a numbered request by prefixing every line with "EN: ",
// which keeps the numbering and makes the mapping visible in assertions.
func echoNumbered(_ int, input translate.TranslationInput) (string, error) {
	var out []string
	for _, line := range strings.Split(input.Source, "\n") {
		match := numberedLine.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		out = append(out, match[1]+". EN: "+match[2])
	}
	return strings.Join(out, "\n"), nil
}

func run(t *testing.T, source string, opts Options, respond func(int, translate.TranslationInput) (string, error)) (*Script, Result, *recordingTranslator) {
	t.Helper()
	parsed := Parse(source)
	translator := &recordingTranslator{respond: respond}
	result, err := Translate(context.Background(), translator, parsed, opts, translate.DiscardReporter{}, nil)
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	return parsed, result, translator
}

const script4 = `@voice am_eric
[0:02] Привет!
$delay(0.5s) Первая возможность - пресеты.
Вот как они выглядят.
[1:20] Дальше снапшоты.
`

func TestTheWholeScriptGoesInOneRequestByDefault(t *testing.T) {
	_, result, translator := run(t, script4, Options{}, echoNumbered)

	if len(translator.inputs) != 1 {
		t.Fatalf("sent %d requests, want 1", len(translator.inputs))
	}
	if len(result.Translations) != 4 {
		t.Errorf("translated %d line(s), want 4", len(result.Translations))
	}
	if len(result.Untranslated) != 0 {
		t.Errorf("untranslated = %v", result.Untranslated)
	}
}

func TestOnlySpokenWordsAreEverSent(t *testing.T) {
	_, _, translator := run(t, script4, Options{}, echoNumbered)

	sent := translator.inputs[0].Source
	for _, timing := range []string{"[0:02]", "$delay", "0.5s", "@voice", "am_eric", "[1:20]"} {
		if strings.Contains(sent, timing) {
			t.Errorf("the request carried %q:\n%s", timing, sent)
		}
	}
	if !strings.Contains(sent, "Привет!") {
		t.Errorf("the request did not carry the speech:\n%s", sent)
	}
}

func TestRequestsAreBuiltWithTheScriptPrompt(t *testing.T) {
	_, _, translator := run(t, script4, Options{Tone: prompt.ToneLiteral}, echoNumbered)

	input := translator.inputs[0]
	if input.Kind != translate.PromptScript {
		t.Errorf("prompt kind = %v, want PromptScript", input.Kind)
	}
	if input.Tone != prompt.ToneLiteral {
		t.Errorf("tone = %v, want literal", input.Tone)
	}
}

func TestChunkingSplitsAndCarriesContextBothWays(t *testing.T) {
	_, _, translator := run(t, script4, Options{ChunkSize: 2}, echoNumbered)

	if len(translator.inputs) != 2 {
		t.Fatalf("sent %d requests, want 2", len(translator.inputs))
	}
	// The first chunk cannot look back, but must be able to look ahead.
	if !strings.Contains(translator.inputs[0].Context, "Дальше снапшоты.") {
		t.Errorf("first chunk had no forward context:\n%s", translator.inputs[0].Context)
	}
	// The second must see the English produced for the first.
	if !strings.Contains(translator.inputs[1].Context, "EN: Привет!") {
		t.Errorf("second chunk had no backward context:\n%s", translator.inputs[1].Context)
	}
}

func TestUserContextReachesEveryRequest(t *testing.T) {
	_, _, translator := run(t, script4, Options{Context: "Orca is a CLI.", ChunkSize: 2}, echoNumbered)

	for i, input := range translator.inputs {
		if !strings.Contains(input.Context, "Orca is a CLI.") {
			t.Errorf("request %d lost the user context:\n%s", i, input.Context)
		}
	}
}

func TestALineTheModelSkipsIsRetriedOnItsOwn(t *testing.T) {
	// The batch answers three of four lines; the fourth has to come back from a
	// request of its own.
	respond := func(call int, input translate.TranslationInput) (string, error) {
		reply, _ := echoNumbered(call, input)
		if call == 1 {
			lines := strings.Split(reply, "\n")
			return strings.Join(lines[:3], "\n"), nil
		}
		return reply, nil
	}
	parsed, result, translator := run(t, script4, Options{}, respond)

	if len(translator.inputs) != 2 {
		t.Fatalf("sent %d requests, want 2 (the batch and one retry)", len(translator.inputs))
	}
	if strings.Count(translator.inputs[1].Source, "\n") != 0 {
		t.Errorf("the retry was not a single line:\n%s", translator.inputs[1].Source)
	}
	if len(result.Translations) != 4 {
		t.Errorf("translated %d line(s), want 4", len(result.Translations))
	}
	// The echo translator keeps the source words, so what marks a line as
	// translated is the prefix it adds - not the absence of Russian.
	if !strings.Contains(parsed.RenderInto(result.Translations), "[1:20] EN: Дальше снапшоты.") {
		t.Errorf("the retried line was left untranslated:\n%s", parsed.RenderInto(result.Translations))
	}
}

func TestALineNoRequestAnswersIsReportedAndLeftAlone(t *testing.T) {
	respond := func(call int, input translate.TranslationInput) (string, error) {
		reply, _ := echoNumbered(call, input)
		if strings.Contains(input.Source, "Дальше снапшоты.") {
			// Answers everything except the line under test, every time.
			var kept []string
			for _, line := range strings.Split(reply, "\n") {
				if !strings.Contains(line, "Дальше снапшоты.") {
					kept = append(kept, line)
				}
			}
			return strings.Join(kept, "\n"), nil
		}
		return reply, nil
	}
	parsed, result, _ := run(t, script4, Options{}, respond)

	if len(result.Untranslated) != 1 || result.Untranslated[0] != 5 {
		t.Fatalf("untranslated = %v, want [5]", result.Untranslated)
	}
	rendered := parsed.RenderInto(result.Translations)
	if !strings.Contains(rendered, "[1:20] Дальше снапшоты.") {
		t.Errorf("the untranslated line lost its timestamp or its text:\n%s", rendered)
	}
	if !strings.Contains(rendered, "[0:02] EN: Привет!") {
		t.Errorf("the lines that did translate were not kept:\n%s", rendered)
	}
}

func TestATransportErrorStopsTheRun(t *testing.T) {
	failure := errors.New("request failed")
	parsed := Parse(script4)
	translator := &recordingTranslator{respond: func(int, translate.TranslationInput) (string, error) {
		return "", failure
	}}
	if _, err := Translate(context.Background(), translator, parsed, Options{}, translate.DiscardReporter{}, nil); !errors.Is(err, failure) {
		t.Fatalf("err = %v, want the transport failure", err)
	}
}

func TestAScriptWithNothingToSayMakesNoRequest(t *testing.T) {
	parsed := Parse("# a note\n@voice am_eric\n$delay 10s\n")
	translator := &recordingTranslator{respond: echoNumbered}
	result, err := Translate(context.Background(), translator, parsed, Options{}, translate.DiscardReporter{}, nil)
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if len(translator.inputs) != 0 {
		t.Errorf("sent %d request(s), want none", len(translator.inputs))
	}
	if len(result.Translations) != 0 {
		t.Errorf("translations = %v", result.Translations)
	}
}

func TestParseNumberedToleratesRealisticReplies(t *testing.T) {
	reply := "Here you go:\n1) First line.\n2. Second line,\n   continued on the next line.\n\n3. Third."
	parsed := parseNumbered(reply)
	want := map[int]string{
		1: "First line.",
		2: "Second line, continued on the next line.",
		3: "Third.",
	}
	for position, expected := range want {
		if parsed[position] != expected {
			t.Errorf("line %d = %q, want %q", position, parsed[position], expected)
		}
	}
	if len(parsed) != len(want) {
		t.Errorf("parsed %d line(s), want %d: %v", len(parsed), len(want), parsed)
	}
}
