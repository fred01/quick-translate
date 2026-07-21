# qt — quick translate

`qt` translates Russian text into clear, natural, polished business English.
It preserves facts, technical meaning, identifiers, scope, and uncertainty,
while turning awkward, colloquial, or heated Russian into calm, professional
English. It works as a Unix-style filter, as an interactive terminal UI, and
comes with a guided setup command.

`qt` is unrelated to the Qt GUI framework — the name stands for "quick
translate".

## Installation

### Homebrew

`qt` ships a Homebrew formula in this repository. Tap it and install:

```sh
brew tap fred01/quick-translate https://github.com/fred01/quick-translate
brew install fred01/quick-translate/qt
```

Use the fully-qualified name `fred01/quick-translate/qt` to avoid a clash with
Homebrew core's `qt` (the Qt GUI framework). Upgrade with
`brew upgrade fred01/quick-translate/qt`.

Prebuilt archives for macOS, Linux, and Windows (amd64 and arm64) are also
attached to each [GitHub release](https://github.com/fred01/quick-translate/releases).

### From source

Requires Go 1.25 or newer.

```sh
go install github.com/fred01/quick-translate/cmd/qt@latest
```

Or build from a local checkout:

```sh
go build -o qt ./cmd/qt
```

## Setup and profiles

`qt` keeps one or more named **profiles** — each a base URL, model, and API
key — with a single **active** profile used by default. This lets you keep,
say, a `litellm` profile and an `nvidia` profile and switch between them.

Configure interactively in a terminal:

```sh
qt setup
```

This opens a profile manager (list, switch active, add, edit, delete). When
adding or editing a profile you provide:

1. **Profile name** — e.g. `litellm`, `nvidia`.
2. **Base URL** — the OpenAI-compatible API root. **It must include the
   `/v1` path** (e.g. `http://localhost:4000/v1`); `qt` never appends `/v1`
   automatically.
3. **Model** — the model name your endpoint serves.
4. **API key** — entered masked. Leave it blank to keep the existing key.
5. **Timeout (s)** — optional per-request timeout in whole seconds. Leave it
   blank for the default of 180; raise it for a large or cold model, lower it
   to fail fast. Editing a profile prefills its current value.

Whenever a profile is saved, `qt` sends one real translation request
(`Привет!`, with no reference context) through the exact same code path used
for normal translation. The profile is only saved if that request succeeds;
a failed test never creates or overwrites your config.

### Managing profiles from the command line

```sh
# Add or update a profile (tests it, saves it, and makes it active):
qt setup add litellm --base-url "http://localhost:4000/v1" --model "translategemma" --api-key "sk-..."
qt setup add nvidia  --base-url "https://integrate.api.nvidia.com/v1" --model "meta/llama-3.1-70b-instruct" --api-key "nvapi-..."

# List profiles (the active one is marked with *):
qt setup list

# Switch the active profile:
qt setup use litellm

# Remove a profile (not the active one, and not the last remaining):
qt setup remove nvidia
```

`add` requires all three of `--base-url`, `--model`, and `--api-key`, and
applies the same test-before-save behavior. These commands write progress to
stderr; only `list` prints to stdout.

### Choosing a profile per translation

The active profile is used by default. Override it for a single invocation
with `--profile` (or the `QT_PROFILE` environment variable) without changing
which profile is active:

```sh
qt --profile nvidia --text "Да, закончил вчера"
cat message.txt | qt --profile litellm
```

## Usage

### `--text`

```sh
qt --text "Да, закончил вчера"
```

### Piping

```sh
cat message.txt | qt
qt < message.txt
```

Reads all of stdin, translates it, and prints exactly the translation
followed by a newline to stdout — nothing else. This is safe to pipe
further:

```sh
cat message.txt | qt | pbcopy
```

### Reference context

An optional `--context` disambiguates pronouns, tense, and terminology using
surrounding conversation — it is never itself translated, quoted, or
answered:

```sh
qt --context "Did you finish the report?" --text "Да, закончил вчера"
# Yes, I finished it yesterday.
```

### Tone

An optional `--tone` selects how faithfully versus how diplomatically the
source's register is rendered:

| Tone         | Behavior                                                                 |
| ------------ | ------------------------------------------------------------------------ |
| `literal`    | Stays as close to the source's wording, directness, and emotional intensity as grammatical English allows. |
| `neutral`    | The default. Clear, polished, professional business English, with slang and strong wording softened. |
| `diplomatic` | Maximally courteous and tactful; reframes complaints and refusals into considerate wording. |

```sh
qt --tone literal --text "Опять сборка упала из-за твоего коммита."
qt --tone diplomatic --text "Опять сборка упала из-за твоего коммита."
```

The meaning, facts, and the essential point are preserved in every tone;
only the register changes. `neutral` is the default, so omitting `--tone`
gives the plainest, most concise rendering (and the fastest, since it
generates the least text); pass `--tone diplomatic` or `--tone literal` for a
more courteous or closer rendering.

### Interactive mode

Running `qt` with no `--text` in a real terminal (stdin and stdout both
terminals) launches an interactive UI with Context, Source, and Translation
panes, a **Tone** selector, plus clickable **Translate**, **Clear**,
**Copy**, and **Setup** buttons. Long translations wrap at word boundaries.

The **Tone** selector above Source is a set of **checkboxes** (Literal,
Neutral, Diplomatic): check one or several. A single **Translate** then
requests every checked tone, running the requests **one at a time**: a
self-hosted model on a single GPU serves one request at a time anyway, so
firing them together only makes them contend and risk timing out, and the
last variant lands no sooner. Sequential requests keep each at full GPU
throughput, and earlier tones' results appear as soon as they finish. When
more than one tone was requested, a **Variants** row of buttons appears above
Translation; click a button (or focus the row and use `←`/`→`) to flip the
Translation pane between the results. `--tone` sets which box starts checked
when the UI opens.

The **Clear** button empties Source and Context and drops the results for a
fresh start (the tone selection and active profile are kept). The **Setup**
button opens the profile manager in place, so you can switch the active
profile or edit profiles without leaving the UI.

An explicit `translate` subcommand is also available and behaves
identically to the root command: `qt translate --text "..."`.

### TUI shortcuts

| Key                        | Action                                     |
| --------------------------- | ------------------------------------------- |
| `Tab` / `Shift+Tab`         | Move focus (Source, Context, Tone, and buttons) |
| `Space`                     | Toggle the tone checkbox under the cursor (when Tone is focused) |
| `←` / `→`                   | Move the tone cursor, or switch the shown variant, depending on focus |
| `Enter`                     | Activate the focused element (e.g. Translate) |
| `Shift+Enter`                | Insert a newline (modern terminals only)   |
| `Alt+Enter`                  | Insert a newline (always works)            |
| `Esc`                        | Cancel an active request, without exiting   |
| `Ctrl+C`                     | Cancel any active request and quit          |
| `PageUp` / `PageDown`        | Scroll the Translation pane                 |
| Mouse click                 | Activate a button, a Variants tab, or focus a field |

The **Copy** button (also reachable with `Tab`) copies the currently shown
variant to the system clipboard via OSC52 once a translation exists.

**Shift+Enter limitation:** distinguishing Shift+Enter from plain Enter
requires a terminal that supports Bubble Tea's keyboard enhancement
protocol. `qt` detects this at startup and updates its own help line to show
whichever binding will actually work in your terminal. `Alt+Enter` is the
portable fallback and always inserts a newline, everywhere.

## Operational status and `--quiet`

In batch mode (`--text` or piped stdin), `qt` writes concise progress and
timing to stderr, for example:

```
qt: sending 19 chars to api.example.com · model=translategemma
qt: received 47 chars in 0.82s
```

`--quiet` suppresses these successful status lines. It never suppresses
errors — failures are always reported, e.g.:

```
qt: request failed after 1.24s: request timed out
```

stdout always contains only the translation on success, and stderr never
contains translated text.

## Configuration

Configuration lives at:

```
<user-config-dir>/qt/config.json
```

(`os.UserConfigDir()` — typically `~/Library/Application Support/qt` on
macOS, `~/.config/qt` on Linux, or `%AppData%\qt` on Windows.) It is plain
JSON with named profiles and an active one:

```json
{
  "active": "litellm",
  "profiles": {
    "litellm": {
      "base_url": "http://localhost:4000/v1",
      "model": "translategemma",
      "api_key": "...",
      "timeout_seconds": 240
    },
    "nvidia": {
      "base_url": "https://integrate.api.nvidia.com/v1",
      "model": "meta/llama-3.1-70b-instruct",
      "api_key": "..."
    }
  }
}
```

`timeout_seconds` is optional and per profile: it bounds each translation
request. Omit it (or set `0`) to use the default of 180 seconds — generous on
purpose, since a self-hosted model on a single GPU can be slow, especially
when cold. Raise it for a large model or long inputs; lower it to fail fast.

An older single-endpoint config (a flat `{base_url, model, api_key}`) is
transparently migrated into a profile named `default` on first load.

API keys are stored in **plaintext**, not encrypted or held in a system
keychain. `qt` restricts filesystem permissions where the platform supports
it (directory mode `0700`, file mode `0600` on Unix); Windows permissions are
best effort.

### Environment overrides

```
QT_PROFILE    select the active profile by name
QT_BASE_URL   override the chosen profile's base URL
QT_MODEL      override the chosen profile's model
QT_API_KEY    override the chosen profile's API key
QT_TIMEOUT    override the per-request timeout, in whole seconds
```

Profile selection precedence is `--profile` flag > `QT_PROFILE` > the stored
active profile. The `QT_BASE_URL`/`QT_MODEL`/`QT_API_KEY`/`QT_TIMEOUT`
variables then override the individual fields of the chosen profile.
`QT_TIMEOUT` must be a positive whole number of seconds; a non-numeric or
non-positive value is ignored, falling back to the profile's `timeout_seconds`
or the default. The three effective field values (base URL, model, API key)
are all required.

## Shell completion

`qt` ships Cobra's built-in completion generator:

```sh
qt completion bash    # or zsh, fish, powershell
```

See `qt completion <shell> --help` for how to load it in your shell.

## Exit codes

- `0` — success
- `1` — configuration, validation, endpoint, or translation failure
- `2` — CLI usage error
- `130` — interrupted with Ctrl+C

## License

Licensed under the Apache License, Version 2.0 — see [LICENSE](LICENSE).
Copyright 2026 Alexey Sviridov.
