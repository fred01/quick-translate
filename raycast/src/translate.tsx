import {
  Action,
  ActionPanel,
  Clipboard,
  Detail,
  Form,
  Icon,
  Toast,
  getPreferenceValues,
  showToast,
  useNavigation,
} from "@raycast/api";
import { execFile } from "child_process";
import { readFileSync } from "fs";
import { homedir } from "os";
import { join } from "path";
import { useState } from "react";
import { promisify } from "util";

const execFileAsync = promisify(execFile);

interface Preferences {
  qtPath: string;
  defaultProfile: string;
}

interface FormValues {
  source: string;
  context: string;
  profile: string;
}

function loadProfiles(): { names: string[]; active: string } {
  try {
    const cfgPath = join(homedir(), "Library", "Application Support", "qt", "config.json");
    const cfg = JSON.parse(readFileSync(cfgPath, "utf8"));
    return { names: Object.keys(cfg.profiles ?? {}), active: cfg.active ?? "" };
  } catch {
    return { names: [], active: "" };
  }
}

function Result({ text, onAgain }: { text: string; onAgain: () => void }) {
  return (
    <Detail
      markdown={text}
      actions={
        <ActionPanel>
          <Action.CopyToClipboard title="Copy Translation" content={text} />
          <Action.Paste title="Paste to Active App" content={text} />
          <Action title="Translate Another" icon={Icon.ArrowLeft} onAction={onAgain} />
        </ActionPanel>
      }
    />
  );
}

export default function Command() {
  const { qtPath, defaultProfile } = getPreferenceValues<Preferences>();
  const { push, pop } = useNavigation();
  const { names, active } = loadProfiles();
  const [isLoading, setIsLoading] = useState(false);

  const preselected =
    defaultProfile && names.includes(defaultProfile) ? defaultProfile : active || names[0] || "";

  async function handleSubmit(values: FormValues) {
    const text = (values.source ?? "").trim();
    if (!text) {
      await showToast({ style: Toast.Style.Failure, title: "Enter some text to translate" });
      return;
    }

    setIsLoading(true);
    const toast = await showToast({ style: Toast.Style.Animated, title: "Translating…" });

    const args = ["--quiet"];
    if (values.profile) args.push("--profile", values.profile);
    const ctx = (values.context ?? "").trim();
    if (ctx) args.push("--context", ctx);
    args.push("--text", text);

    try {
      const { stdout } = await execFileAsync(qtPath, args, { timeout: 120_000 });
      const translation = stdout.trim();
      await Clipboard.copy(translation);
      toast.style = Toast.Style.Success;
      toast.title = "Copied to clipboard";
      push(<Result text={translation} onAgain={pop} />);
    } catch (err) {
      const e = err as { stderr?: string; message?: string };
      toast.style = Toast.Style.Failure;
      toast.title = "Translation failed";
      toast.message = (e.stderr || e.message || String(err)).trim();
    } finally {
      setIsLoading(false);
    }
  }

  return (
    <Form
      isLoading={isLoading}
      actions={
        <ActionPanel>
          <Action.SubmitForm title="Translate" icon={Icon.Globe} onSubmit={handleSubmit} />
        </ActionPanel>
      }
    >
      <Form.TextArea id="source" title="Russian" placeholder="Введите русский текст…" autoFocus />
      <Form.TextArea id="context" title="Context" placeholder="Optional context to disambiguate the translation" />
      {names.length > 0 ? (
        <Form.Dropdown id="profile" title="Profile" defaultValue={preselected}>
          {names.map((n) => (
            <Form.Dropdown.Item key={n} value={n} title={n === active ? `${n} (active)` : n} />
          ))}
        </Form.Dropdown>
      ) : (
        <Form.Description title="Profile" text="Using qt's active profile" />
      )}
    </Form>
  );
}
