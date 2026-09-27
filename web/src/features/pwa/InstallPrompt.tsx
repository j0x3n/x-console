import { useEffect, useState } from "react";
import { Download, X } from "lucide-react";
import "./pwa.css";

interface InstallEvent extends Event {
  prompt: () => Promise<void>;
  userChoice: Promise<{ outcome: "accepted" | "dismissed" }>;
}

export default function InstallPrompt() {
  const [event, setEvent] = useState<InstallEvent | null>(null);
  const [dismissed, setDismissed] = useState(false);
  const ios =
    /iPhone|iPad|iPod/.test(navigator.userAgent) ||
    (/Macintosh/.test(navigator.userAgent) && navigator.maxTouchPoints > 1);
  const installed =
    window.matchMedia("(display-mode: standalone)").matches ||
    (navigator as Navigator & { standalone?: boolean }).standalone === true;
  useEffect(() => {
    const onPrompt = (value: Event) => {
      value.preventDefault();
      setEvent(value as InstallEvent);
    };
    const onInstalled = () => setEvent(null);
    window.addEventListener("beforeinstallprompt", onPrompt);
    window.addEventListener("appinstalled", onInstalled);
    return () => {
      window.removeEventListener("beforeinstallprompt", onPrompt);
      window.removeEventListener("appinstalled", onInstalled);
    };
  }, []);
  if ((!event && !ios) || installed || dismissed) return null;
  const install = async () => {
    if (!event) return;
    try {
      await event.prompt();
      await event.userChoice;
    } catch {
      // The browser may dismiss the install prompt without a choice.
    } finally {
      setEvent(null);
    }
  };
  return (
    <div className="pwa-install" role="status">
      <span>
        {event
          ? "把 X Console 安装到设备，打开更方便。"
          : "在浏览器中点分享，再点“添加到主屏幕”。"}
      </span>
      {event && (
        <button className="xc-btn small primary" onClick={() => void install()}>
          <Download size={14} /> 安装
        </button>
      )}
      <button
        className="xc-btn small ghost"
        aria-label="关闭安装提示"
        onClick={() => setDismissed(true)}
      >
        <X size={14} />
      </button>
    </div>
  );
}
