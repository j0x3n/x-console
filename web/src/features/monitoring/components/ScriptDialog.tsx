import { useEffect, useState, type FormEvent } from "react";
import { errorMessage } from "../../../api/client";
import { withElevation } from "../../../auth/elevation";
import Dialog from "../../../components/ui/Dialog";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import { useSaveScript, type Script, type ScriptShell } from "../api";
import HostPicker from "./HostPicker";

interface Props {
  open: boolean;
  onClose: () => void;
  script?: Script | null;
  onSaved?: (s: Script) => void;
}

const shells: { id: ScriptShell; label: string }[] = [
  { id: "bash", label: "bash" },
  { id: "sh", label: "sh" },
  { id: "powershell", label: "PowerShell" },
];

export default function ScriptDialog({
  open,
  onClose,
  script,
  onSaved,
}: Props) {
  const t = useT();
  const save = useSaveScript();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [shell, setShell] = useState<ScriptShell>("bash");
  const [body, setBody] = useState("");
  const [hosts, setHosts] = useState<string[]>([]);
  const [timeout, setTimeoutSec] = useState("300");
  const [error, setError] = useState("");

  useEffect(() => {
    if (!open) return;
    setError("");
    setName(script?.name ?? "");
    setDescription(script?.description ?? "");
    setShell(script?.shell ?? "bash");
    setBody(script?.body ?? "");
    setHosts(script?.defaultHostIds ?? []);
    setTimeoutSec(String(script?.timeoutSeconds ?? 300));
  }, [open, script]);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!name.trim()) return setError(t("Please enter a name"));
    if (!body.trim()) return setError(t("The script is empty"));
    const seconds = Number(timeout);
    if (!seconds || seconds > 1800)
      return setError(t("Timeout must be 1 to 1800 seconds"));
    const fields = {
      name: name.trim(),
      description: description.trim(),
      shell,
      body,
      defaultHostIds: hosts,
      timeoutSeconds: seconds,
    };
    try {
      let saved: Script;
      if (script) {
        // 只发改过的内容。改脚本内容和执行一样敏感，服务端会要求再次验证。
        const { body: newBody, shell: newShell, ...rest } = fields;
        const patch = {
          ...rest,
          ...(newBody !== script.body ? { body: newBody } : {}),
          ...(newShell !== script.shell ? { shell: newShell } : {}),
        };
        saved = await withElevation(() =>
          save.mutateAsync({ id: script.id, patch }),
        );
      } else {
        saved = await save.mutateAsync({ create: fields });
      }
      toast(t("Saved"));
      onSaved?.(saved);
      onClose();
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={script ? t("Edit script") : t("New script")}
      wide
    >
      <form onSubmit={submit}>
        <div className="monitoring-form-row">
          <label className="xc-field">
            <span>{t("Name")}</span>
            <input
              className="xc-input"
              value={name}
              onChange={(e) => setName(e.target.value)}
              maxLength={100}
              autoFocus
            />
          </label>
          <label className="xc-field monitoring-narrow-field">
            <span>{t("Shell")}</span>
            <select
              className="xc-select"
              value={shell}
              onChange={(e) => setShell(e.target.value as ScriptShell)}
            >
              {shells.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.label}
                </option>
              ))}
            </select>
          </label>
        </div>
        <label className="xc-field">
          <span>{t("Description")}</span>
          <input
            className="xc-input"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder={t("optional")}
          />
        </label>
        <label className="xc-field">
          <span>{t("Script")}</span>
          <textarea
            className="xc-textarea xc-mono monitoring-code"
            value={body}
            onChange={(e) => setBody(e.target.value)}
            rows={10}
            spellCheck={false}
            placeholder={
              shell === "powershell"
                ? "Get-Service | Where Status -eq Running"
                : "df -h\nuptime"
            }
          />
          <small>
            {t("bash and sh run on Linux. PowerShell runs on Windows.")}
          </small>
        </label>
        <div className="xc-field">
          <span>{t("Default machines")}</span>
          <HostPicker value={hosts} onChange={setHosts} />
        </div>
        <label className="xc-field monitoring-narrow-field">
          <span>{t("Timeout (seconds)")}</span>
          <input
            className="xc-input"
            inputMode="numeric"
            value={timeout}
            onChange={(e) => setTimeoutSec(e.target.value.replace(/\D/g, ""))}
          />
        </label>
        {error && <p className="xc-error-text">{error}</p>}
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button
            type="submit"
            className="xc-btn primary"
            disabled={save.isPending}
          >
            {t("Save")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
