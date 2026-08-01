package prompt

import "strings"

// scriptRules adapts the base prompt to a narration script: a numbered batch of
// lines that will be read aloud and dropped onto a soundtrack.
//
// Timing is not mentioned as something to preserve, because the caller never
// sends it - the model only ever sees the words. What it does need to know is
// that the lines are numbered, that the numbering is the contract, and that
// each line has to fit the slot the original occupied.
const scriptRules = `Format — the source is a numbered list of narration lines:
- Each line begins with a number, a period, and a space, then the line's text.
- Return exactly one output line per input line, with the same number, in the same order, in the same "N. text" form.
- Never merge two lines into one, never split one line into two, and never add, drop, reorder, or renumber lines. The numbering is how the translation is put back together; if it does not match, the result is discarded.
- Translate only the text after the number. Do not translate, repeat, or comment on the numbers themselves.
- If a line is a fragment that continues the previous one, translate it as a fragment. Do not complete it into a sentence using the neighbouring lines.
- Return the numbered lines and nothing else: no preamble, no summary, no blank lines between them, no Markdown.

Narration — this text will be spoken aloud over a screen recording:
- Write English that sounds natural when read out, not English that reads well on a page. Prefer plain words and short clauses over subordinate constructions.
- Keep each line close to the length of its source. Every line is played into a slot of fixed length, and a translation that runs long overruns the picture. Where English is naturally shorter, let it be shorter; do not pad.
- Do not add filler, transitions, or connective phrases that are not in the source, even where they would make the narration flow better.
- Expand nothing for the listener's benefit: no restating, no clarifying asides, no "in other words".
- Keep product names, commands, and flags exactly as written. If the source spells a name out letter by letter for the voice, keep it spelled out the same way.
`

// scriptContextRules replaces the reference-context rules for script mode: the
// context here is usually the surrounding lines of the same script, sent so a
// chunk in the middle knows what it is continuing.
const scriptContextRules = `Reference-context rules:
- The reference context is neighbouring narration, provided only so the lines you translate follow on from it naturally in tense, terminology, and register.
- Translate only the numbered source lines.
- Do not output, translate, quote, summarize, or answer the reference context.
- Do not import facts from the reference context unless the source lines clearly refer to them.
`

// BuildScript constructs the prompt for one batch of narration lines. source
// must already be a numbered block, one line per entry; context is neighbouring
// narration and may be empty. tone selects register exactly as it does for
// ordinary translation.
func BuildScript(source, context string, tone Tone) string {
	var b strings.Builder
	b.WriteString(basePrompt)
	if d := toneDirective(tone); d != "" {
		b.WriteString("\n")
		b.WriteString(d)
	}
	b.WriteString("\n")
	b.WriteString(scriptRules)
	if strings.TrimSpace(context) != "" {
		b.WriteString("\n")
		b.WriteString(scriptContextRules)
		b.WriteString("\n--- BEGIN REFERENCE CONTEXT ---\n")
		b.WriteString(context)
		b.WriteString("\n--- END REFERENCE CONTEXT ---\n")
	}
	b.WriteString("\n--- BEGIN SOURCE TEXT ---\n")
	b.WriteString(source)
	b.WriteString("\n--- END SOURCE TEXT ---\n")
	return b.String()
}
