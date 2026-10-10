import { RefreshCw, Send } from "lucide-react";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { relativeTime } from "../../lib/time";
import type { AIConfigHost, AIConfigHostStatus } from "./api";
import {
  HOST_STATE_LABELS,
  ITEM_LABELS,
  TOOL_LABELS,
  hostTone,
  reasonLabel,
} from "./format";

export default function HostsCard({
  hosts,
  selected,
  onToggle,
  statuses,
  checking,
  canApply,
  applying,
  onCheck,
  onApply,
}: {
  hosts: AIConfigHost[];
  selected: string[];
  onToggle: (id: string, on: boolean) => void;
  statuses: AIConfigHostStatus[] | undefined;
  checking: boolean;
  canApply: boolean;
  applying: boolean;
  onCheck: () => void;
  onApply: (hostId?: string) => void;
}) {
  const t = useT();
  const language = useLanguage();
  const byId = new Map((statuses ?? []).map((s) => [s.hostId, s]));
  return (
    <section
      className="xc-card aiconfig-hosts"
      aria-label={t("Deliver to machines")}
    >
      <div className="xc-card-head">
        <h2>{t("Deliver to machines")}</h2>
        <span className="xc-spacer" />
        <button
          type="button"
          className="xc-btn ghost small"
          disabled={checking || selected.length === 0}
          onClick={onCheck}
        >
          <RefreshCw size={14} /> {t("Check again")}
        </button>
        <button
          type="button"
          className="xc-btn primary small"
          disabled={!canApply || applying || selected.length === 0}
          title={canApply ? undefined : t("Save the changes first")}
          onClick={() => onApply()}
        >
          <Send size={14} /> {t("Deliver to all")}
        </button>
      </div>
      {hosts.length === 0 ? (
        <p className="aiconfig-empty">
          {t("No machine is paired yet")}
          <br />
          <small>
            {t("Pair a server or a computer in Settings, then come back here.")}
          </small>
        </p>
      ) : (
        <ul className="aiconfig-host-list">
          {hosts.map((h) => {
            const on = selected.includes(h.id);
            const st = on ? byId.get(h.id) : undefined;
            const state = h.supported ? st?.state : "unsupported";
            return (
              <li key={h.id} className="aiconfig-host">
                <div className="aiconfig-host-main">
                  <label className="xc-check aiconfig-host-check">
                    <input
                      type="checkbox"
                      checked={on}
                      disabled={!h.supported && !on}
                      onChange={(e) => onToggle(h.id, e.target.checked)}
                    />
                    <span>{h.name}</span>
                  </label>
                  <span className={`xc-dot ${h.online ? "ok" : ""}`} />
                  <small>{h.online ? t("Online") : t("Offline")}</small>
                  <span className="xc-spacer" />
                  {on && !st && checking && (
                    <small className="aiconfig-checking">
                      <span className="xc-spinner aiconfig-spinner" />{" "}
                      {t("Checking")}
                    </small>
                  )}
                  {on && state && (
                    <span className={`xc-badge ${hostTone(state)}`}>
                      {t(HOST_STATE_LABELS[state])}
                    </span>
                  )}
                  {on && h.supported && h.online && (
                    <button
                      type="button"
                      className="xc-btn small"
                      disabled={!canApply || applying}
                      title={canApply ? undefined : t("Save the changes first")}
                      aria-label={`${t("Deliver")} ${h.name}`}
                      onClick={() => onApply(h.id)}
                    >
                      {t("Deliver")}
                    </button>
                  )}
                </div>
                {!h.supported && (
                  <small className="aiconfig-hint">
                    {t(
                      "Needs Claude Code or Codex on the machine, and an up-to-date agent.",
                    )}
                  </small>
                )}
                {st && st.state !== "ok" && st.state !== "offline" && (
                  <ul className="aiconfig-items">
                    {st.error && <li className="aiconfig-error">{st.error}</li>}
                    {st.items
                      .filter((it) => it.state !== "ok" || it.error)
                      .map((it) => (
                        <li key={`${it.tool}-${it.item}`}>
                          <strong>
                            {TOOL_LABELS[it.tool]} · {t(ITEM_LABELS[it.item])}
                          </strong>
                          {it.state === "absent" ? (
                            <span> {t("Not installed")}</span>
                          ) : (
                            <span>
                              {" "}
                              {t(reasonLabel(it))}
                              {it.names?.length
                                ? `（${it.names.join("、")}）`
                                : ""}
                            </span>
                          )}
                          {it.error && (
                            <span className="aiconfig-error"> {it.error}</span>
                          )}
                        </li>
                      ))}
                  </ul>
                )}
                {st?.checkedAt && (
                  <small className="aiconfig-hint">
                    {t("Last checked")} {relativeTime(st.checkedAt, language)}
                  </small>
                )}
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}
