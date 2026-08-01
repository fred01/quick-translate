// Package script parses narration scripts - the format speech-translator's
// `stt` writes and its `tts` reads - so only the spoken words are translated.
//
// A script places lines on a soundtrack. The placement is written as timestamps
// and delays, and it must survive translation byte for byte: a model that
// helpfully rewrites `[1:20]` or translates `$delay` produces a file that no
// longer renders. So the model never sees them. Each line is split into a
// prefix, which is carried through untouched, and the spoken text, which is all
// that is ever sent anywhere:
//
//	[1:20] Первая возможность - пресеты.
//	^^^^^^ ^--------------------------^
//	prefix  text
//
// Reassembly is prefix + translation, so the timing is not preserved by asking
// nicely - it is preserved because nothing else could happen to it.
package script

import (
	"regexp"
	"strings"
)

var (
	// `[1:20]`, `[0:01:20]`, `[1:20.5]`, `[12]`, `[12.5]`, `[+2.5]`
	timestampPattern = regexp.MustCompile(`^\[\s*(?:\+\s*)?(?:\d+:)?(?:\d+:)?\d+(?:\.\d+)?\s*\]\s*`)
	// `@voice am_eric`, `@duration 19:45`
	directivePattern = regexp.MustCompile(`^@\w+`)
	// `$delay 2s`, `$delay(500ms)`, and the same without the `$`.
	delayPattern = regexp.MustCompile(`^\$?delay\s*(?:\(\s*\d+(?:\.\d+)?\s*(?:ms|s|m)?\s*\)|\s+\d+(?:\.\d+)?\s*(?:ms|s|m)?)\s*`)
)

// Line is one line of a script, split so that translation can only ever touch
// Text. Raw is the line exactly as it was read, including its indentation.
type Line struct {
	Number int
	Raw    string
	// Prefix is everything before the spoken words: indentation, stacked
	// delays, and a timestamp. Empty for a line that simply follows the one
	// above.
	Prefix string
	// Text is the spoken words, and the only thing sent to a model. Empty for
	// a comment, a directive, a blank line, or a bare delay.
	Text string
}

// Spoken reports whether this line carries words that a voice will read.
func (l Line) Spoken() bool { return l.Text != "" }

// Render returns the line with text substituted for its spoken words. A line
// that is not spoken is returned exactly as it was read.
func (l Line) Render(text string) string {
	if !l.Spoken() {
		return l.Raw
	}
	return l.Prefix + text
}

// Script is a parsed narration script: every line of the input, in order.
type Script struct {
	Lines []Line
}

// SpokenIndexes returns the positions in Lines of the lines that carry words.
func (s *Script) SpokenIndexes() []int {
	indexes := make([]int, 0, len(s.Lines))
	for i, line := range s.Lines {
		if line.Spoken() {
			indexes = append(indexes, i)
		}
	}
	return indexes
}

// SpokenCount returns how many lines carry words.
func (s *Script) SpokenCount() int { return len(s.SpokenIndexes()) }

// Parse splits text into lines, marking which of them are spoken. It never
// fails: a line it cannot recognise as structure is spoken text, which is the
// safe assumption - the worst case is translating something that did not need
// it, rather than dropping a line from the soundtrack.
func Parse(text string) *Script {
	// A trailing newline would otherwise become a phantom final line, and be
	// written back as a doubled one.
	body := strings.TrimSuffix(text, "\n")
	raws := strings.Split(body, "\n")
	if body == "" {
		raws = nil
	}

	lines := make([]Line, 0, len(raws))
	for i, raw := range raws {
		lines = append(lines, parseLine(i+1, raw))
	}
	return &Script{Lines: lines}
}

func parseLine(number int, raw string) Line {
	line := Line{Number: number, Raw: raw}

	trimmed := strings.TrimLeft(raw, " \t")
	indent := raw[:len(raw)-len(trimmed)]
	if trimmed == "" || strings.HasPrefix(trimmed, "#") || directivePattern.MatchString(trimmed) {
		return line
	}

	// Structure comes first and may stack: `$delay 1s $delay 500ms [1:20] text`
	// is all legal, and all of it belongs in the prefix.
	prefix := indent
	rest := trimmed
	for {
		match := delayPattern.FindString(rest)
		if match == "" {
			break
		}
		prefix += match
		rest = rest[len(match):]
	}
	if match := timestampPattern.FindString(rest); match != "" {
		prefix += match
		rest = rest[len(match):]
	}

	if strings.TrimSpace(rest) == "" {
		// A delay or a timestamp with nothing after it: structure only.
		return line
	}
	line.Prefix = prefix
	line.Text = rest
	return line
}

// RenderInto rebuilds the script with translations substituted for the spoken
// lines. translations is keyed by index into Lines; a line missing from the map
// keeps its original text, so a partial translation degrades to a partly
// translated script rather than to a broken one.
func (s *Script) RenderInto(translations map[int]string) string {
	var b strings.Builder
	for i, line := range s.Lines {
		if translated, ok := translations[i]; ok && line.Spoken() {
			b.WriteString(line.Render(translated))
		} else {
			b.WriteString(line.Raw)
		}
		b.WriteString("\n")
	}
	return b.String()
}
