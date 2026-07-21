# Quick Translate — Raycast extension

A Raycast extension that wraps the [`qt`](../README.md) CLI: press a global
hotkey, get a full-size form to type Russian text, and translate it into
polished business English without leaving the keyboard.

The command **Translate RU→EN** opens a form with:

- **Russian** — the text to translate (autofocused).
- **Context** — optional reference context passed to `qt --context`.
- **Tone** — how faithfully versus how diplomatically the source's register is
  rendered, passed to `qt --tone`: **Neutral** (the default, polished business
  English), **Diplomatic** (most courteous), or **Literal** (closest to the
  source).
- **Profile** — which `qt` profile to use; the list is read live from `qt`'s
  config, preselecting the `Default profile` preference (default `LiteLLM`).

On submit it runs `qt`, copies the translation to the clipboard, and shows it
in a detail view with **Copy** and **Paste** actions.

## Preferences

| Preference        | Default                  | Purpose                                   |
| ----------------- | ------------------------ | ----------------------------------------- |
| `qt binary path`  | `/opt/homebrew/bin/qt`   | Absolute path to the `qt` executable.     |
| `Default profile` | `LiteLLM`                | `qt` profile preselected in the form.     |

## Install (local development)

Requires Node and a running Raycast.

```sh
cd raycast
npm install
npm run dev      # imports the extension into Raycast; Ctrl+C once it appears
```

The extension stays installed in Raycast after the dev server stops (it loads
from the built bundle). `npm run build` type-checks and bundles it.

## Assign a global hotkey

Raycast stores hotkeys internally, so this is done in the UI:
**Raycast → Settings → Extensions → Quick Translate → Translate RU→EN →
Record Hotkey**.
