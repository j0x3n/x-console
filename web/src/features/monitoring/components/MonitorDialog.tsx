import { useEffect, useState, type FormEvent } from "react";
import { errorMessage } from "../../../api/client";
import Dialog from "../../../components/ui/Dialog";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import { useSaveMonitor, type Monitor, type MonitorKind } from "../api";

interface Props {
  open: boolean;
  onClose: () => void;
  /** 编辑时传入 */
  monitor?: Monitor | null;
  /** 新建时可选的类型：网站页只有 http，证书页是 tls 和 domain */
  kinds: MonitorKind[];
  onSaved?: (m: Monitor) => void;
}

const kindLabels: Record<MonitorKind, string> = {
  http: "Website",
  tls: "Certificate",
  domain: "Domain",
};

const placeholders: Record<MonitorKind, string> = {
  http: "https://example.com/health",
  tls: "example.com 或 example.com:8443",
  domain: "example.com",
};

// 默认检查间隔，和服务端一致。
const defaultIntervals: Record<MonitorKind, number> = { http: 60, tls: 21600, domain: 86400 };

export default function MonitorDialog({ open, onClose, monitor, kinds, onSaved }: Props) {
  const t = useT();
  const save = useSaveMonitor();
  const [kind, setKind] = useState<MonitorKind>(kinds[0]);
  const [name, setName] = useState("");
  const [target, setTarget] = useState("");
  const [interval, setIntervalSec] = useState("");
  const [expected, setExpected] = useState("");
  const [keyword, setKeyword] = useState("");
  const [timeout, setTimeoutMs] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    if (!open) return;
    setError("");
    const k = monitor?.kind ?? kinds[0];
    setKind(k);
    setName(monitor?.name ?? "");
    setTarget(monitor?.target ?? "");
    setIntervalSec(String(monitor?.intervalSeconds ?? defaultIntervals[k]));
    setExpected(monitor && monitor.expectedStatus > 0 ? String(monitor.expectedStatus) : "");
    setKeyword(monitor?.keyword ?? "");
    setTimeoutMs(String(monitor?.timeoutMs ?? 10000));
  }, [open, monitor, kinds]);

  const changeKind = (k: MonitorKind) => {
    setKind(k);
    if (!monitor) setIntervalSec(String(defaultIntervals[k]));
  };

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!name.trim()) return setError(t("Please enter a name"));
    if (!target.trim()) return setError(t("Please enter what to check"));
    const fields = {
      name: name.trim(),
      target: target.trim(),
      intervalSeconds: Number(interval) || defaultIntervals[kind],
      expectedStatus: Number(expected) || 0,
      keyword: keyword.trim(),
      timeoutMs: Number(timeout) || 10000,
    };
    try {
      const saved = monitor
        ? await save.mutateAsync({ id: monitor.id, patch: fields })
        : await save.mutateAsync({ create: { kind, ...fields } });
      toast(t("Saved"));
      onSaved?.(saved);
      onClose();
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const minutes = Math.round((Number(interval) || 0) / 60);
  return (
    <Dialog open={open} onClose={onClose} title={monitor ? t("Edit monitor") : t("New monitor")}>
      <form onSubmit={submit}>
        {!monitor && kinds.length > 1 && (
          <div className="xc-field">
            <span>{t("Type")}</span>
            <div className="monitoring-segment" role="radiogroup">
              {kinds.map((k) => (
                <button
                  key={k}
                  type="button"
                  role="radio"
                  aria-checked={kind === k}
                  className={kind === k ? "active" : ""}
                  onClick={() => changeKind(k)}
                >
                  {t(kindLabels[k])}
                </button>
              ))}
            </div>
          </div>
        )}
        <label className="xc-field">
          <span>{t("Name")}</span>
          <input className="xc-input" value={name} onChange={(e) => setName(e.target.value)} maxLength={100} autoFocus />
        </label>
        <label className="xc-field">
          <span>{kind === "http" ? t("URL") : kind === "tls" ? t("Host") : t("Domain")}</span>
          <input
            className="xc-input"
            value={target}
            onChange={(e) => setTarget(e.target.value)}
            placeholder={placeholders[kind]}
            spellCheck={false}
          />
          {kind === "domain" && <small>{t("The expiry date comes from RDAP. It is checked once a day.")}</small>}
        </label>
        {kind === "http" && (
          <div className="monitoring-form-row">
            <label className="xc-field">
              <span>{t("Expected status")}</span>
              <input
                className="xc-input"
                inputMode="numeric"
                value={expected}
                onChange={(e) => setExpected(e.target.value.replace(/\D/g, ""))}
                placeholder="200–399"
              />
            </label>
            <label className="xc-field">
              <span>{t("Keyword")}</span>
              <input className="xc-input" value={keyword} onChange={(e) => setKeyword(e.target.value)} placeholder={t("optional")} />
            </label>
          </div>
        )}
        <div className="monitoring-form-row">
          <label className="xc-field">
            <span>{t("Check every (seconds)")}</span>
            <input
              className="xc-input"
              inputMode="numeric"
              value={interval}
              onChange={(e) => setIntervalSec(e.target.value.replace(/\D/g, ""))}
            />
            {minutes >= 1 && <small>≈ {minutes >= 60 ? `${Math.round(minutes / 60)} h` : `${minutes} min`}</small>}
          </label>
          <label className="xc-field">
            <span>{t("Timeout (ms)")}</span>
            <input
              className="xc-input"
              inputMode="numeric"
              value={timeout}
              onChange={(e) => setTimeoutMs(e.target.value.replace(/\D/g, ""))}
            />
          </label>
        </div>
        {kind === "http" && <p className="monitoring-hint">{t("Two failed checks in a row send an alert. You get another message when it is back.")}</p>}
        {kind !== "http" && (
          <p className="monitoring-hint">
            {kind === "tls"
              ? t("You get a reminder 14, 7 and 3 days before the certificate expires.")
              : t("You get a reminder 30 and 7 days before the domain expires.")}
          </p>
        )}
        {error && <p className="xc-error-text">{error}</p>}
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button type="submit" className="xc-btn primary" disabled={save.isPending}>
            {t("Save")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
