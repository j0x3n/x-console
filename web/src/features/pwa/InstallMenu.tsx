import { useEffect } from "react";
import { createPortal } from "react-dom";
import { Download, Share, SquarePlus } from "lucide-react";
import Dialog from "../../components/ui/Dialog";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { isIOS, registerServiceWorker, useInstall } from "./install";
import "./i18n";
import "./pwa.css";

/*
 * 个人菜单里的“安装应用”（B22，原来在页头和设置 → 通用）。已经装好时不显示。
 * 浏览器能直接装时点一下就装；不能时弹窗说明怎么装。
 */
export function InstallMenuItem({ onDone }: { onDone: () => void }) {
  const t = useT();
  const prompt = useInstall((s) => s.prompt);
  const installed = useInstall((s) => s.installed);
  const install = useInstall((s) => s.install);
  if (installed) return null;
  return (
    <button
      className="profile-menu-item"
      role="menuitem"
      onClick={async () => {
        onDone();
        if (prompt) {
          if (await install()) toast(t("Installed"));
        } else useInstall.setState({ helpOpen: true });
      }}
    >
      <Download size={15} />
      <span>{t("Install app")}</span>
    </button>
  );
}

/**
 * 怎么安装的说明弹窗。放在菜单外面，菜单关了弹窗还在。
 * 挂载时顺便注册 service worker（推送通知也用它）。
 */
export function InstallHelpDialog() {
  const t = useT();
  const open = useInstall((s) => s.helpOpen);
  useEffect(registerServiceWorker, []);
  if (!open) return null;
  const close = () => useInstall.setState({ helpOpen: false });
  return createPortal(
    <Dialog
      open
      onClose={close}
      title={t("Install app")}
      footer={
        <button className="xc-btn primary" onClick={close}>
          {t("Got it")}
        </button>
      }
    >
      {isIOS() ? (
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
          打开，地址栏右边会出现安装按钮。安卓上在浏览器菜单里选“安装应用”或“添加到主屏幕”。装好以后打开就是独立窗口，没有浏览器地址栏。
        </p>
      )}
    </Dialog>,
    document.body,
  );
}
