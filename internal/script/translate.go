package script

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/fred01/quick-translate/internal/prompt"
	"github.com/fred01/quick-translate/internal/translate"
)

// Scripts are translated in one request by default, and the default is not a
// compromise: a twenty minute narration is around 150 spoken lines and 10k
// characters, which is a few thousand tokens - nothing for any model this tool
// talks to. Splitting it would cost real quality, because narration is one
// argument developed across its lines: a term introduced in the opening decides
// how it is rendered forty lines later, and a fragment only makes sense as a
// continuation of the line above it.
//
// Chunking therefore exists only for scripts that genuinely will not fit, and
// is opt-in through --chunk. When it is on, every chunk is given the lines
// around it - the English already produced before, and the Russian still to
// come - so the seams cost as little as they can.
const (
	// contextLinesBefore is how many already-translated lines precede a chunk,
	// so it continues them in the same tense, terminology, and register.
	contextLinesBefore = 8
	// contextLinesAfter is how many untranslated source lines follow a chunk.
	// Fewer, because what comes next mostly settles pronouns and tense.
	contextLinesAfter = 5
)

// numberedLine matches `12. text`, tolerating `12) text` and leading
// whitespace, which models produce often enough to be worth accepting.
var numberedLine = regexp.MustCompile(`^\s*(\d+)\s*[.)]\s?(.*)$`)

// Options configures a script translation.
type Options struct {
	Tone prompt.Tone
	// Context is extra reference material supplied by the user, prepended to
	// the neighbouring-lines context of every request.
	Context string
	// ChunkSize splits the script into requests of this many spoken lines.
	// Zero - the default - sends the whole script as one request, which is
	// what keeps the narration coherent; see the note above.
	ChunkSize int
}

// Result is the outcome of translating a script.
type Result struct {
	// Translations maps an index into Script.Lines to its English text. Lines
	// absent from the map were not translated and keep their source text.
	Translations map[int]string
	// Untranslated lists the source line numbers that no request returned, in
	// order. Empty on a clean run.
	Untranslated []int
}

// Progress receives one call per completed chunk, for a caller that wants to
// show something during a long script.
type Progress interface {
	Chunk(done, total int)
}

type noProgress struct{}

func (noProgress) Chunk(int, int) {}

// Translate translates every spoken line of s and returns the translations
// keyed by line index.
//
// A response that cannot be matched back to its numbering is not thrown away:
// each line it failed to account for is retried on its own, where the numbering
// is trivial and a model has nothing to lose track of. Whatever still fails is
// reported in Result.Untranslated and left in Russian, because a script with
// two untranslated lines is something a person can finish by hand, and an error
// is not.
func Translate(
	ctx context.Context,
	translator translate.Translator,
	s *Script,
	opts Options,
	reporter translate.Reporter,
	progress Progress,
) (Result, error) {
	if progress == nil {
		progress = noProgress{}
	}

	indexes := s.SpokenIndexes()
	result := Result{Translations: make(map[int]string, len(indexes))}
	if len(indexes) == 0 {
		return result, nil
	}

	chunkSize := opts.ChunkSize
	if chunkSize <= 0 {
		chunkSize = len(indexes)
	}
	chunks := (len(indexes) + chunkSize - 1) / chunkSize

	for c := 0; c < chunks; c++ {
		start := c * chunkSize
		end := min(start+chunkSize, len(indexes))
		batch := indexes[start:end]

		// A single-request run needs no neighbouring context: the whole script
		// is the request.
		contextText := opts.Context
		if chunks > 1 {
			contextText = result.chunkContext(s, indexes, start, end, opts.Context)
		}

		translated, err := translateBatch(ctx, translator, s, batch, opts, reporter, contextText)
		if err != nil {
			return result, err
		}
		for index, text := range translated {
			result.Translations[index] = text
		}

		// Anything the response did not account for gets its own request. One
		// line numbered "1." leaves nothing to misalign.
		for _, index := range batch {
			if _, ok := result.Translations[index]; ok {
				continue
			}
			single, err := translateBatch(ctx, translator, s, []int{index}, opts, reporter,
				result.lineContext(s, indexes, index, opts.Context))
			if err != nil {
				return result, err
			}
			if text, ok := single[index]; ok {
				result.Translations[index] = text
			} else {
				result.Untranslated = append(result.Untranslated, s.Lines[index].Number)
			}
		}
		progress.Chunk(c+1, chunks)
	}
	return result, nil
}

// chunkContext describes what surrounds a chunk: the English already produced
// before it, and the Russian that follows it.
func (r Result) chunkContext(s *Script, indexes []int, start, end int, extra string) string {
	var parts []string
	if trimmed := strings.TrimSpace(extra); trimmed != "" {
		parts = append(parts, trimmed)
	}

	before := indexes[max(0, start-contextLinesBefore):start]
	var english []string
	for _, index := range before {
		if text, ok := r.Translations[index]; ok {
			english = append(english, text)
		}
	}
	if len(english) > 0 {
		parts = append(parts, "The narration immediately before these lines, already translated:\n"+
			strings.Join(english, "\n"))
	}

	after := indexes[end:min(len(indexes), end+contextLinesAfter)]
	var russian []string
	for _, index := range after {
		russian = append(russian, s.Lines[index].Text)
	}
	if len(russian) > 0 {
		parts = append(parts, "The narration that follows these lines, still untranslated:\n"+
			strings.Join(russian, "\n"))
	}
	return strings.Join(parts, "\n\n")
}

// lineContext is the context for retrying one line alone: its neighbours on
// both sides, in whatever language they are currently in. A fragment retried
// with no context around it is the one case where a model reliably completes it
// into a sentence it should not have been.
func (r Result) lineContext(s *Script, indexes []int, index int, extra string) string {
	position := 0
	for i, candidate := range indexes {
		if candidate == index {
			position = i
			break
		}
	}
	return r.chunkContext(s, indexes, position, position+1, extra)
}

// translateBatch sends one numbered block and maps the reply back onto the
// script's line indexes. A line the reply does not cover is simply absent from
// the returned map; that is the caller's cue to retry it.
func translateBatch(
	ctx context.Context,
	translator translate.Translator,
	s *Script,
	batch []int,
	opts Options,
	reporter translate.Reporter,
	contextText string,
) (map[int]string, error) {
	var b strings.Builder
	for position, index := range batch {
		fmt.Fprintf(&b, "%d. %s\n", position+1, s.Lines[index].Text)
	}

	input := translate.TranslationInput{
		Source:  strings.TrimSuffix(b.String(), "\n"),
		Context: contextText,
		Tone:    opts.Tone,
		Kind:    translate.PromptScript,
	}
	reply, err := translator.Translate(ctx, input, reporter)
	if err != nil {
		return nil, err
	}

	translated := make(map[int]string, len(batch))
	for position, text := range parseNumbered(reply) {
		if position < 1 || position > len(batch) {
			continue
		}
		if text = strings.TrimSpace(text); text != "" {
			translated[batch[position-1]] = text
		}
	}
	return translated, nil
}

// parseNumbered reads a numbered reply into position -> text. A line that does
// not start with a number continues the one before it, which is how a model
// renders a translation that happens to contain a newline; anything before the
// first number is preamble and is dropped.
func parseNumbered(reply string) map[int]string {
	parsed := make(map[int]string)
	current := 0
	for _, raw := range strings.Split(reply, "\n") {
		if match := numberedLine.FindStringSubmatch(raw); match != nil {
			position, err := strconv.Atoi(match[1])
			if err == nil {
				current = position
				parsed[current] = match[2]
				continue
			}
		}
		if current == 0 {
			continue
		}
		if trimmed := strings.TrimSpace(raw); trimmed != "" {
			parsed[current] += " " + trimmed
		}
	}
	return parsed
}
