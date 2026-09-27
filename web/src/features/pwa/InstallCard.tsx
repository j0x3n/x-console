import { Download, Share, SquarePlus } from "lucide-react";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { isIOS, useInstall } from "./install";
import "./i18n";
import "./pwa.css";

/** 设置 → 通用里的“安装应用”。说明各个浏览器怎么装。 */
export default function InstallCard() {
  const t = useT();
  const prompt = useInstall((s) => s.prompt);
  const installed = useInstall((s) => s.installed);
  const dismissed = useInstall((s) => s.dismissed);
  const install = useInstall((s) => s.install);
  const dismiss = useInstall((s) => s.dismiss);
  return (
    <div className="xc-card">
      <div className="xc-card-head">
        <h2>{t("Install app")}</h2>
        {installed && <span className="xc-badge ok">{t("Installed")}</span>}
      </div>
      {installed ? (
        <p className="pwa-note">已经装好了，可以从桌面或主屏幕直接打开。</p>
      ) : prompt ? (
        <>
          <p className="pwa-note">
            装到桌面或主屏幕后，打开就是独立窗口，没有浏览器地址栏。
          </p>
          <div className="pwa-actions">
            <button
              className="xc-btn primary"
              onClick={async () => {
                if (await install()) toast(t("Installed"));
              }}
            >
              <Download size={15} /> {t("Install app")}
            </button>
            {!dismissed && (
              <button type="button" className="xc-btn ghost" onClick={dismiss}>
                {t("Hide the header button")}
              </button>
            )}
          </div>
          {dismissed && (
            <p className="pwa-note small">
              页头的安装按钮已经关掉了，在这里还能装。
            </p>
          )}
        </>
      ) : isIOS() ? (
        <ol className="pwa-steps">
          <li>
            用 Safari 打开这个页面，点底部的{" "}
            <Share size={14} aria-label="分享" /> 分享按钮。
          </li>
          <li>
            选 <SquarePlus size={14} aria-label="添加" /> “添加到主屏幕”。
          </li>
        </ol>
      ) : (
        <p className="pwa-note">
          用 Chrome 或 Edge
          打开，地址栏右边会出现安装按钮。安卓上在浏览器菜单里选“安装应用”或“添加到主屏幕”。
        </p>
      )}
    </div>
  );
}
