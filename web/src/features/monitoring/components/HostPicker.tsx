import { useT } from "../../../contexts/LanguageContext";
import { useHostOptions } from "../api";

/** 选择机器的勾选列表。离线的机器也能选，执行时会记录失败原因。 */
export default function HostPicker({ value, onChange }: { value: string[]; onChange: (ids: string[]) => void }) {
  const t = useT();
  const hosts = useHostOptions();
  if (hosts.isPending) return <small className="xc-muted">{t("Loading")}</small>;
  if (hosts.isError || hosts.data.length === 0) return <small className="xc-muted">{t("No machines yet")}</small>;
  const toggle = (id: string) => onChange(value.includes(id) ? value.filter((x) => x !== id) : [...value, id]);
  return (
    <div className="monitoring-hosts">
      {hosts.data.map((h) => (
        <label key={h.id} className={`monitoring-host${value.includes(h.id) ? " checked" : ""}`}>
          <input type="checkbox" checked={value.includes(h.id)} onChange={() => toggle(h.id)} />
          <span className={`xc-dot ${h.online ? "ok" : ""}`} />
          <span>{h.name}</span>
        </label>
      ))}
    </div>
  );
}
