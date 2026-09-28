import { useMemo } from "react";
import { FileDiff } from "lucide-react";
import { useT } from "../../../contexts/LanguageContext";
import type { TaskDiff } from "../api";
import { parseDiff } from "../logic";

const STATUS_NAMES: Record<string, string> = {
  A: "File added",
  M: "File modified",
  D: "File deleted",
  T: "File type changed",
};

/** 改动文件列表和 diff。文件多或很长时默认折叠。 */
export default function DiffView({ diff }: { diff: TaskDiff }) {
  const t = useT();
  const parsed = useMemo(() => parseDiff(diff.diff), [diff.diff]);
  const byPath = useMemo(
    () => new Map(parsed.map((f) => [f.path, f])),
    [parsed],
  );
  const totalLines = parsed.reduce((n, f) => n + f.lines.length, 0);
  const collapse = parsed.length > 12 || totalLines > 1500;
  if (diff.files.length === 0) {
    return <p className="xc-muted">{t("No changes.")}</p>;
  }
  return (
    <div className="coding-diff">
      <ul className="coding-files">
        {diff.files.map((f) => (
          <li key={f.path}>
            <span
              className={`coding-file-status s-${f.status}`}
              title={t(STATUS_NAMES[f.status] ?? f.status)}
            >
              {f.status}
            </span>
            <a
              href={`#diff-${encodeURIComponent(f.path)}`}
              className="coding-file-path"
            >
              {f.path}
            </a>
            {f.binary ? (
              <span className="xc-muted">{t("binary")}</span>
            ) : (
              <span className="coding-file-stat">
                <span className="add">+{f.additions}</span>{" "}
                <span className="del">-{f.deletions}</span>
              </span>
            )}
          </li>
        ))}
      </ul>
      {diff.truncated && (
        <p className="coding-note">
          {t("The diff is larger than 2 MB. Only the first part is shown.")}
        </p>
      )}
      {diff.files.map((f) => {
        const file = byPath.get(f.path);
        return (
          <details
            key={f.path}
            id={`diff-${encodeURIComponent(f.path)}`}
            className="coding-diff-file"
            open={!collapse || (file?.lines.length ?? 0) < 80}
          >
            <summary>
              <FileDiff size={14} /> <span className="xc-mono">{f.path}</span>
            </summary>
            {!file || file.binary || file.lines.length === 0 ? (
              <p className="xc-muted coding-diff-empty">
                {f.binary || file?.binary
                  ? t("Binary file, not shown.")
                  : t("No text changes to show.")}
              </p>
            ) : (
              <div className="coding-diff-body">
                <table>
                  <tbody>
                    {file.lines.map((l, i) => (
                      <tr key={i} className={`l-${l.kind}`}>
                        <td className="no">{l.oldNo ?? ""}</td>
                        <td className="no">{l.newNo ?? ""}</td>
                        <td className="code">
                          <span className="sign">
                            {l.kind === "add"
                              ? "+"
                              : l.kind === "del"
                                ? "-"
                                : " "}
                          </span>
                          {l.text}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </details>
        );
      })}
    </div>
  );
}
