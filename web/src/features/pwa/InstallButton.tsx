import { useEffect } from "react";
import { Download } from "lucide-react";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { registerServiceWorker, useInstall } from "./install";
import "./i18n";

/*
 * 页头的“安装应用”按钮。只在浏览器允许安装、还没装、没被关掉时出现。
 * 挂载时顺便注册 service worker。
 */
export default function InstallButton() {
  const t = useT();
  const prompt = useInstall((s) => s.prompt);
  const installed = useInstall((s) => s.installed);
  const dismissed = useInstall((s) => s.dismissed);
  const install = useInstall((s) => s.install);
  useEffect(registerServiceWorker, []);
  if (!prompt || installed || dismissed) return null;
  return (
    <button
      className="icon-button"
      title={t("Install app")}
      aria-label={t("Install app")}
      onClick={async () => {
        if (await install()) toast(t("Installed"));
      }}
    >
      <Download size={16} />
    </button>
  );
}
