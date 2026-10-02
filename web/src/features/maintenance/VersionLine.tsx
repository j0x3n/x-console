import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { formatDate, formatTime } from "../../lib/time";
import { useServerVersion, versionMismatch, webVersion } from "./api";
import "./i18n";

/**
 * 设置页底部一行小字（B79）：版本 a1b2c3d · 10月1日 21:52 构建。
 * 点一下复制版本号。前后端版本不一致时提示刷新。
 */
export default function VersionLine() {
  const t = useT();
  const language = useLanguage();
  const server = useServerVersion();
  const built = new Date(webVersion.builtAt);
  const stale = versionMismatch(webVersion.version, server.data);
  const copy = () =>
    navigator.clipboard
      ?.writeText(webVersion.version)
      .then(() => toast(t("Version copied")))
      .catch(() => {});
  return (
    <div className="settings-version">
      <button type="button" onClick={() => void copy()} title={t("Copy")}>
        {t("Version {v}").replace("{v}", webVersion.version)} ·{" "}
        {t("built {time}").replace(
          "{time}",
          `${formatDate(built, language)} ${formatTime(built, language)}`,
        )}
      </button>
      {stale && (
        <button
          type="button"
          className="settings-version-stale"
          onClick={() => window.location.reload()}
        >
          {t("Page is out of date. Refresh it.")}
        </button>
      )}
    </div>
  );
}
