import { usageTone } from "../lib";

/** 使用率小条。颜色表示状态，旁边总有数字，不只靠颜色。 */
export default function UsageBar({
  label,
  value,
  detail,
}: {
  label: string;
  value: number | undefined;
  detail?: string;
}) {
  const tone = usageTone(value);
  return (
    <div className="servers-usage">
      <div className="servers-usage-top">
        <span>{label}</span>
        <span className="servers-usage-value">
          {value === undefined ? "—" : `${value.toFixed(0)}%`}
          {detail && <small> · {detail}</small>}
        </span>
      </div>
      <span
        className={`servers-bar ${tone}`}
        role="progressbar"
        aria-label={label}
        aria-valuenow={value ?? 0}
        aria-valuemin={0}
        aria-valuemax={100}
      >
        <i style={{ width: `${Math.min(100, value ?? 0)}%` }} />
      </span>
    </div>
  );
}
