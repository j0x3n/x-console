import { useMemo, useState } from "react";
import { Link } from "react-router";
import { FileCode2, Play, TerminalSquare } from "lucide-react";
import { errorMessage } from "../../../api/client";
import { withElevation } from "../../../auth/elevation";
import { confirmAction } from "../../../components/ui/ConfirmDialog";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import { useRunScript, useScripts, type Script } from "../../monitoring/api";
import type { HostDetail } from "../api";

/** 看起来会删数据、关机、格式化的命令，运行前再确认一次。 */
const RISKY =
  /\brm\s+-[a-z]*r[a-z]*f|\brm\s+-[a-z]*f[a-z]*r|\bshutdown\b|\breboot\b|\bpoweroff\b|\bmkfs\b|\bdd\s+[^\n]*of=|\bformat-volume\b|\bremove-item\b[^\n]*-recurse|\bstop-computer\b|\brestart-computer\b/i;

export function isRisky(body: string): boolean {
  return RISKY.test(body);
}

/** 把脚本变成可以直接送进交互式终端的一段输入（B28）。 */
export function terminalInput(script: Pick<Script, "shell" | "body">): string {
  if (script.shell === "powershell") {
    // PowerShell 的 -EncodedCommand 要 UTF-16LE 的 base64。
    const bytes = new Uint8Array(script.body.length * 2);
    for (let i = 0; i < script.body.length; i++) {
      const c = script.body.charCodeAt(i);
      bytes[i * 2] = c & 0xff;
      bytes[i * 2 + 1] = c >> 8;
    }
    let bin = "";
    bytes.forEach((b) => (bin += String.fromCharCode(b)));
    return `powershell -NoProfile -EncodedCommand ${btoa(bin)}\r`;
  }
  // heredoc 加引号，脚本里的 $、反引号、引号都不会被当前 shell 解释。
  let tag = "XC_EOF";
  while (script.body.includes(tag)) tag += "_";
  const body = script.body.replace(/\r\n/g, "\n").replace(/\n?$/, "\n");
  return `${script.shell} <<'${tag}'\n${body}${tag}\n`;
}

function fitsHost(script: Script, host: HostDetail) {
  const windows = host.os === "windows";
  return windows
    ? script.shell === "powershell"
    : script.shell !== "powershell";
}

/** 终端右边的“脚本”面板：和这台机器关联的监控脚本，一键运行。 */
export default function TerminalScripts({
  host,
  connected,
  send,
}: {
  host: HostDetail;
  connected: boolean;
  send: (text: string) => void;
}) {
  const t = useT();
  const scripts = useScripts();
  const run = useRunScript();
  const [all, setAll] = useState(false);
  const list = useMemo(() => {
    const items = (scripts.data ?? []).filter((s) => fitsHost(s, host));
    const related = items.filter((s) => s.defaultHostIds.includes(host.id));
    return { related, others: items.filter((s) => !related.includes(s)) };
  }, [scripts.data, host]);
  const shown = all ? [...list.related, ...list.others] : list.related;

  const confirmRisky = async (s: Script) =>
    !isRisky(s.body) ||
    confirmAction({
      title: `${t("Run")}“${s.name}”？`,
      description: t(
        "The script looks like it deletes data or restarts the machine.",
      ),
      confirmLabel: t("Run"),
    });

  const inTerminal = async (s: Script) => {
    if (!(await confirmRisky(s))) return;
    send(terminalInput(s));
  };
  const inBackground = async (s: Script) => {
    if (!(await confirmRisky(s))) return;
    try {
      await withElevation(() =>
        run.mutateAsync({ id: s.id, hostIds: [host.id] }),
      );
      toast(t("Started. See the result in Monitoring → Scripts."));
    } catch (e) {
      toast({ message: errorMessage(e), tone: "error" });
    }
  };

  return (
    <aside className="servers-scripts" aria-label={t("Scripts")}>
      <div className="servers-scripts-head">
        <strong>{t("Scripts")}</strong>
        {list.others.length > 0 && (
          <label className="xc-check">
            <input
              type="checkbox"
              checked={all}
              onChange={(e) => setAll(e.target.checked)}
            />
            <span>{t("Show all")}</span>
          </label>
        )}
      </div>
      {scripts.isError ? (
        <p className="xc-muted">{errorMessage(scripts.error)}</p>
      ) : shown.length === 0 ? (
        <p className="xc-muted servers-scripts-empty">
          {scripts.isPending
            ? t("Loading")
            : t("No scripts linked to this machine yet.")}
        </p>
      ) : (
        <ul>
          {shown.map((s) => (
            <li key={s.id}>
              <div
                className="servers-scripts-name"
                title={s.description || s.name}
              >
                <FileCode2 size={13} />
                <span>{s.name}</span>
                <small className="xc-muted">{s.shell}</small>
              </div>
              <div className="servers-scripts-actions">
                <button
                  className="xc-btn small"
                  disabled={!connected}
                  title={
                    connected ? undefined : t("Connect the terminal first")
                  }
                  onClick={() => inTerminal(s)}
                >
                  <TerminalSquare size={13} /> {t("Run in terminal")}
                </button>
                <button
                  className="xc-btn small ghost"
                  disabled={!host.online || run.isPending}
                  onClick={() => inBackground(s)}
                >
                  <Play size={13} /> {t("Run in background")}
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}
      <Link className="servers-scripts-manage" to="/monitoring/scripts">
        {t("Manage scripts")}
      </Link>
    </aside>
  );
}
