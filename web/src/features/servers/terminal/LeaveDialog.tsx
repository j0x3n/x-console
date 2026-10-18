import { useEffect, useState } from "react";
import Dialog from "../../../components/ui/Dialog";
import { useT } from "../../../contexts/LanguageContext";
import "./i18n";

type T = ReturnType<typeof useT>;

export function evictedMessage(t: T, hostName: string) {
  return t(
    "Disconnected the terminal on {name}. Up to 4 stay in the background.",
  ).replace("{name}", hostName);
}

/**
 * B249：终端连着时离开服务器页、电脑页先问。点遮罩或按 Esc 等于不走。
 * 勾上“以后都这样”时记进偏好，设置 → 通用 → 终端里能改回来。
 */
export default function LeaveDialog({
  open,
  onChoose,
  onCancel,
}: {
  open: boolean;
  onChoose: (choice: "keep" | "close", remember: boolean) => void;
  onCancel: () => void;
}) {
  const t = useT();
  const [remember, setRemember] = useState(false);
  useEffect(() => {
    if (open) setRemember(false);
  }, [open]);
  return (
    <Dialog
      open={open}
      onClose={onCancel}
      title={t("The terminal is still connected")}
      description={t(
        "A terminal left in the background stays connected. Running commands keep running.",
      )}
      footer={
        <>
          <button
            className="xc-btn"
            onClick={() => onChoose("close", remember)}
          >
            {t("Disconnect")}
          </button>
          <button
            className="xc-btn primary"
            autoFocus
            onClick={() => onChoose("keep", remember)}
          >
            {t("Keep in background")}
          </button>
        </>
      }
    >
      <label className="xc-check">
        <input
          type="checkbox"
          checked={remember}
          onChange={(e) => setRemember(e.target.checked)}
        />
        {t("Always do this, don't ask again")}
      </label>
    </Dialog>
  );
}
