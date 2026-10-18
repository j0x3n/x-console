import { useEffect, useRef } from "react";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import {
  disconnect,
  expireSessions,
  hideDock,
  setDock,
  useTerminals,
} from "./store";
import "./i18n";

/**
 * B249：留在后台的终端放在这里，挂在全局浮层里，切换模块时不卸载。
 * 每分钟看一次，后台超过 30 分钟没看的终端断开。
 */
export default function TerminalDock() {
  const t = useT();
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    hideDock(el);
    setDock(el);
    return () => {
      setDock(null);
      // 登录后的布局卸载（退出登录）时，终端一个都不留
      for (const id of Object.keys(useTerminals.getState().sessions))
        disconnect(id);
    };
  }, []);
  useEffect(() => {
    const id = setInterval(() => {
      for (const s of expireSessions())
        toast(
          t(
            "The terminal on {name} was idle in the background for 30 minutes and was disconnected.",
          ).replace("{name}", s.hostName),
        );
    }, 60_000);
    return () => clearInterval(id);
  }, [t]);
  return <div ref={ref} />;
}
