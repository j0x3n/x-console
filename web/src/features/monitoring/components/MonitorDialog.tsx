import { useEffect, useState, type FormEvent } from "react";
import { errorMessage } from "../../../api/client";
import Dialog from "../../../components/ui/Dialog";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import {
  useMonitors,
  useSaveMonitor,
  type Monitor,
  type MonitorKind,
  type MonitorPatch,
} from "../api";
import { companionMonitors, normalizeSiteUrl, registrableDomain } from "../lib";

interface Props {
  open: boolean;
  onClose: () => void;
  /** 编辑时传入 */
  monitor?: Monitor | null;
  /** 新建时：网站页是 http，证书页是 tls 和 domain（一起建，B50） */
  kinds: MonitorKind[];
  onSaved?: (m: Monitor) => void;
}

const placeholders: Record<MonitorKind, string> = {
  http: "example.com 或 https://example.com/health",
  tls: "example.com 或 example.com:8443",
  domain: "example.com",
};

// 默认检查间隔，和服务端一致。
const defaultIntervals: Record<MonitorKind, number> = {
  http: 60,
  tls: 21600,
  domain: 86400,
};

export default function MonitorDialog({
  open,
  onClose,
  monitor,
  kinds,
  onSaved,
}: Props) {
  const t = useT();
  const save = useSaveMonitor();
  const all = useMonitors().data ?? [];
  // 新建证书和域名时不再分类型，填一个域名，勾选要查的（B50）。
  const expiryOnly = !monitor && !kinds.includes("http");
  const kind: MonitorKind = monitor?.kind ?? (expiryOnly ? "domain" : "http");
  const [name, setName] = useState("");
  const [target, setTarget] = useState("");
  const [interval, setIntervalSec] = useState("");
  const [expected, setExpected] = useState("");
  const [keyword, setKeyword] = useState("");
  const [timeout, setTimeoutMs] = useState("");
  const [withTls, setWithTls] = useState(true);
  const [withDomain, setWithDomain] = useState(true);
  const [manual, setManual] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!open) return;
    setError("");
    setName(monitor?.name ?? "");
    setTarget(monitor?.target ?? "");
    setIntervalSec(String(monitor?.intervalSeconds ?? defaultIntervals[kind]));
    setExpected(
      monitor && monitor.expectedStatus > 0
        ? String(monitor.expectedStatus)
        : "",
    );
    setKeyword(monitor?.keyword ?? "");
    setTimeoutMs(String(monitor?.timeoutMs ?? 10000));
    setWithTls(true);
    setWithDomain(true);
    setManual(monitor?.manualExpiresAt ?? "");
  }, [open, monitor, kind]);

  const isHttpUrl =
    kind === "http" &&
    normalizeSiteUrl(target).toLowerCase().startsWith("http://");

  const createExtra = async (base: string, label: string) => {
    const extra = companionMonitors(base, all, {
      tls: withTls,
      domain: withDomain,
    });
    let failed = 0;
    for (const x of extra) {
      try {
        await save.mutateAsync({ create: { ...x, name: label } });
      } catch {
        failed++;
      }
    }
    return { added: extra.length - failed, failed };
  };

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!target.trim()) return setError(t("Please enter what to check"));
    setError("");
    setBusy(true);
    try {
      if (monitor) {
        if (!name.trim()) return setError(t("Please enter a name"));
        const patch: MonitorPatch = {
          name: name.trim(),
          target: kind === "http" ? normalizeSiteUrl(target) : target.trim(),
          intervalSeconds: Number(interval) || defaultIntervals[kind],
          expectedStatus: Number(expected) || 0,
          keyword: keyword.trim(),
          timeoutMs: Number(timeout) || 10000,
        };
        if (kind === "domain") {
          if (manual) patch.manualExpiresAt = manual;
          else if (monitor.manualExpiresAt) patch.clearManualExpiry = true;
        }
        const saved = await save.mutateAsync({ id: monitor.id, patch });
        toast(t("Saved"));
        onSaved?.(saved);
        onClose();
        return;
      }
      if (expiryOnly) {
        if (!withTls && !withDomain)
          return setError(t("Pick at least one thing to check"));
        const label = name.trim() || registrableDomain(target);
        const res = await createExtra(target, label);
        if (res.added === 0 && res.failed === 0)
          return setError(t("These are already monitored"));
        if (res.failed > 0)
          toast({ message: t("Some checks were not added"), tone: "error" });
        else toast(t("Saved"));
        onClose();
        return;
      }
      const url = normalizeSiteUrl(target);
      const label = name.trim() || new URL(url).hostname;
      const saved = await save.mutateAsync({
        create: {
          kind: "http",
          name: label,
          target: url,
          intervalSeconds: Number(interval) || defaultIntervals.http,
          expectedStatus: Number(expected) || 0,
          keyword: keyword.trim(),
          timeoutMs: Number(timeout) || 10000,
        },
      });
      const res = await createExtra(url, label);
      if (res.failed > 0)
        toast({ message: t("Some checks were not added"), tone: "error" });
      else toast(t("Saved"));
      onSaved?.(saved);
      onClose();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const minutes = Math.round((Number(interval) || 0) / 60);
  const targetLabel =
    kind === "http" ? t("URL") : kind === "tls" ? t("Host") : t("Domain");
  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={
        monitor
          ? t("Edit monitor")
          : expiryOnly
            ? t("New monitor")
            : t("New website")
      }
    >
      <form onSubmit={submit}>
        <label className="xc-field">
          <span>{targetLabel}</span>
          <input
            className="xc-input"
            value={target}
            onChange={(e) => setTarget(e.target.value)}
            onBlur={() =>
              kind === "http" &&
              !expiryOnly &&
              setTarget(normalizeSiteUrl(target))
            }
            placeholder={placeholders[kind]}
            spellCheck={false}
            autoCapitalize="off"
            autoFocus
          />
          {kind === "http" && !monitor && (
            <small>{t("Without http:// it uses https:// for you.")}</small>
          )}
        </label>
        <label className="xc-field">
          <span>{t("Name")}</span>
          <input
            className="xc-input"
            value={name}
            onChange={(e) => setName(e.target.value)}
            maxLength={100}
            placeholder={monitor ? undefined : t("Uses the domain if empty")}
          />
        </label>
        {!monitor && (
          <div className="xc-field">
            <span>{expiryOnly ? t("What to check") : t("Also check")}</span>
            <div className="monitoring-remind">
              <label className="xc-check">
                <input
                  type="checkbox"
                  checked={withTls && !isHttpUrl}
                  disabled={isHttpUrl}
                  onChange={(e) => setWithTls(e.target.checked)}
                />
                <span>{t("Certificate expiry")}</span>
              </label>
              <label className="xc-check">
                <input
                  type="checkbox"
                  checked={withDomain}
                  onChange={(e) => setWithDomain(e.target.checked)}
                />
                <span>{t("Domain expiry")}</span>
              </label>
            </div>
            <small>
              {t(
                "Certificates are reminded 14, 7 and 3 days ahead. Domains 30 and 7 days ahead.",
              )}
            </small>
          </div>
        )}
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
              <input
                className="xc-input"
                value={keyword}
                onChange={(e) => setKeyword(e.target.value)}
                placeholder={t("optional")}
              />
            </label>
          </div>
        )}
        {!expiryOnly && (
          <div className="monitoring-form-row">
            <label className="xc-field">
              <span>{t("Check every (seconds)")}</span>
              <input
                className="xc-input"
                inputMode="numeric"
                value={interval}
                onChange={(e) =>
                  setIntervalSec(e.target.value.replace(/\D/g, ""))
                }
              />
              {minutes >= 1 && (
                <small>
                  ≈{" "}
                  {minutes >= 60
                    ? `${Math.round(minutes / 60)} h`
                    : `${minutes} min`}
                </small>
              )}
            </label>
            <label className="xc-field">
              <span>{t("Timeout (ms)")}</span>
              <input
                className="xc-input"
                inputMode="numeric"
                value={timeout}
                onChange={(e) =>
                  setTimeoutMs(e.target.value.replace(/\D/g, ""))
                }
              />
            </label>
          </div>
        )}
        {kind === "domain" && monitor && (
          <label className="xc-field">
            <span>{t("Expiry date (manual)")}</span>
            <input
              className="xc-input"
              type="date"
              value={manual}
              onChange={(e) => setManual(e.target.value)}
            />
            <small>
              {t(
                "Used only when RDAP and WHOIS both have no answer. Leave empty to look it up.",
              )}
            </small>
          </label>
        )}
        {kind === "http" && (
          <p className="monitoring-hint">
            {t(
              "Two failed checks in a row send an alert. You get another message when it is back.",
            )}
          </p>
        )}
        {monitor && kind !== "http" && (
          <p className="monitoring-hint">
            {kind === "tls"
              ? t(
                  "You get a reminder 14, 7 and 3 days before the certificate expires.",
                )
              : t(
                  "You get a reminder 30 and 7 days before the domain expires.",
                )}
          </p>
        )}
        {error && <p className="xc-error-text">{error}</p>}
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button
            type="submit"
            className="xc-btn primary"
            disabled={busy || save.isPending}
          >
            {t("Save")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
