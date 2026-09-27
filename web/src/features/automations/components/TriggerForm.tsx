import { Copy } from "lucide-react";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import type { Catalog, Trigger } from "../api";
import { defaultTrigger, TRIGGER_TYPES } from "../logic";

export default function TriggerForm({
  value,
  topics,
  onChange,
}: {
  value: Trigger;
  topics: Catalog["topics"];
  onChange: (t: Trigger) => void;
}) {
  const t = useT();
  const set = (patch: Partial<Trigger>) => onChange({ ...value, ...patch });
  return (
    <div className="auto-trigger">
      <div className="auto-types" role="radiogroup" aria-label={t("Trigger")}>
        {TRIGGER_TYPES.map((tt) => (
          <button
            key={tt.type}
            type="button"
            role="radio"
            aria-checked={value.type === tt.type}
            className={value.type === tt.type ? "on" : ""}
            onClick={() =>
              value.type !== tt.type && onChange(defaultTrigger(tt.type))
            }
          >
            {tt.label}
          </button>
        ))}
      </div>

      {value.type === "schedule" && (
        <label className="xc-field">
          <span>{t("Cron expression")}</span>
          <input
            className="xc-input xc-mono"
            value={value.cron ?? ""}
            onChange={(e) => set({ cron: e.target.value })}
            placeholder="0 9 * * 1-5"
          />
          <small>
            分 时 日 月 周，按你的时区。例：0 9 * * 1-5 是工作日早上 9 点。
          </small>
        </label>
      )}

      {value.type === "event" && (
        <>
          <label className="xc-field">
            <span>{t("Event topic")}</span>
            <input
              className="xc-input xc-mono"
              list="auto-topics"
              value={value.topic ?? ""}
              onChange={(e) => set({ topic: e.target.value })}
              placeholder="monitor.down"
            />
            <datalist id="auto-topics">
              {topics.map((tp) => (
                <option key={tp.topic} value={tp.topic}>
                  {tp.title}
                </option>
              ))}
            </datalist>
            <small>按前缀匹配。比如 issue. 会匹配所有 Issue 的事件。</small>
          </label>
          <label className="xc-field">
            <span>{t("Match (optional)")}</span>
            <input
              className="xc-input xc-mono"
              value={value.match ?? ""}
              onChange={(e) => set({ match: e.target.value || undefined })}
              placeholder='data.hostId == "abc"'
            />
          </label>
        </>
      )}

      {value.type === "metric" && (
        <div className="auto-row">
          <label className="xc-field">
            <span>{t("Server")}</span>
            <input
              className="xc-input"
              value={value.hostId ?? ""}
              onChange={(e) => set({ hostId: e.target.value || undefined })}
              placeholder={t("Any server")}
            />
          </label>
          <label className="xc-field">
            <span>{t("Metric")}</span>
            <select
              className="xc-select"
              value={value.metric ?? "cpu"}
              onChange={(e) =>
                set({ metric: e.target.value as Trigger["metric"] })
              }
            >
              <option value="cpu">CPU</option>
              <option value="mem">{t("Memory")}</option>
              <option value="disk">{t("Disk")}</option>
            </select>
          </label>
          <label className="xc-field auto-narrow">
            <span>{t("Compare")}</span>
            <select
              className="xc-select"
              value={value.op ?? ">"}
              onChange={(e) => set({ op: e.target.value as Trigger["op"] })}
            >
              {[">", ">=", "<", "<="].map((o) => (
                <option key={o} value={o}>
                  {o}
                </option>
              ))}
            </select>
          </label>
          <label className="xc-field auto-narrow">
            <span>{t("Threshold %")}</span>
            <input
              className="xc-input"
              type="number"
              value={value.value ?? ""}
              onChange={(e) =>
                set({
                  value:
                    e.target.value === "" ? undefined : Number(e.target.value),
                })
              }
            />
          </label>
        </div>
      )}

      {value.type === "ha_state" && (
        <div className="auto-row">
          <label className="xc-field">
            <span>{t("Entity ID")}</span>
            <input
              className="xc-input xc-mono"
              value={value.entityId ?? ""}
              onChange={(e) => set({ entityId: e.target.value })}
              placeholder="binary_sensor.front_door"
            />
          </label>
          <label className="xc-field">
            <span>{t("Becomes (optional)")}</span>
            <input
              className="xc-input xc-mono"
              value={value.to ?? ""}
              onChange={(e) => set({ to: e.target.value || undefined })}
              placeholder="on"
            />
          </label>
        </div>
      )}

      {value.type === "webhook" &&
        (value.webhookPath ? (
          <div className="xc-field">
            <span>{t("Webhook address")}</span>
            <div className="auto-webhook">
              <code>{`${location.origin}/api/v1${value.webhookPath}`}</code>
              <button
                type="button"
                className="xc-btn small"
                onClick={() =>
                  navigator.clipboard
                    .writeText(`${location.origin}/api/v1${value.webhookPath}`)
                    .then(() => toast(t("Link copied")))
                }
              >
                <Copy size={13} />
              </button>
            </div>
            <small>
              用 POST
              调这个地址，请求体会作为触发数据。地址本身就是密钥，不要外传。
            </small>
          </div>
        ) : (
          <p className="auto-muted">保存后会生成一个地址。</p>
        ))}
    </div>
  );
}
