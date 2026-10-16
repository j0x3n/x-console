import { WEEKDAY_LABELS, windowError } from "./lib";

/** 检查时段：每天几点到几点，周几检查（B236）。都不填就全天检查。 */
export default function WindowFields({
  from,
  to,
  days,
  onChange,
}: {
  from: string;
  to: string;
  days: number[];
  onChange: (v: { from: string; to: string; days: number[] }) => void;
}) {
  const error = windowError(from, to);
  const set = from !== "" || to !== "" || days.length > 0;
  return (
    <div className="xc-field full">
      <label htmlFor="watcher-window-from">检查时段</label>
      <div className="watchers-window">
        <input
          id="watcher-window-from"
          className="xc-input"
          type="time"
          aria-label="开始时间"
          value={from}
          onChange={(e) => onChange({ from: e.target.value, to, days })}
        />
        <span>到</span>
        <input
          className="xc-input"
          type="time"
          aria-label="结束时间"
          value={to}
          onChange={(e) => onChange({ from, to: e.target.value, days })}
        />
        {set && (
          <button
            type="button"
            className="xc-btn ghost small"
            onClick={() => onChange({ from: "", to: "", days: [] })}
          >
            清除
          </button>
        )}
      </div>
      <div className="watchers-window-days" role="group" aria-label="检查日期">
        {WEEKDAY_LABELS.map((label, i) => (
          <label key={label} className="xc-check">
            <input
              type="checkbox"
              checked={days.includes(i + 1)}
              onChange={(e) =>
                onChange({
                  from,
                  to,
                  days: e.target.checked
                    ? [...days, i + 1].sort((a, b) => a - b)
                    : days.filter((d) => d !== i + 1),
                })
              }
            />
            <span>周{label}</span>
          </label>
        ))}
      </div>
      {error ? (
        <small className="xc-error-text">{error}</small>
      ) : (
        <small>
          只在这个时间和这几天检查，其他时候不检查，也不提醒你。都不填就全天检查。结束比开始早表示跨过午夜。
        </small>
      )}
    </div>
  );
}
