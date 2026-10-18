import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { useBlocker, useLocation } from "react-router";
import "@xterm/xterm/css/xterm.css";
import { Plug, RotateCw } from "lucide-react";
import { errorMessage } from "../../../api/client";
import { withElevation } from "../../../auth/elevation";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import { usePreferencesStore } from "../../../stores/preferences-store";
import { ensureElevated, type HostDetail } from "../api";
import {
  fitSession,
  leaveSession,
  openSession,
  sendInput,
  showSession,
} from "../terminal/sessions";
import { disconnect, useTerminals, type TermInfo } from "../terminal/store";
import LeaveDialog, { evictedMessage } from "../terminal/LeaveDialog";
import TerminalScripts from "./TerminalScripts";

/** 回到这台机器终端页的地址。电脑页靠 ?host= 选机器。 */
function terminalPath(pathname: string, hostId: string) {
  const id = encodeURIComponent(hostId);
  return pathname.startsWith("/pc")
    ? `/pc/terminal?host=${id}`
    : `/servers/${id}/terminal`;
}

/**
 * xterm.js 终端。二进制帧是终端数据，文本帧是窗口大小。
 * 打开前先确认 5 分钟内验证过，否则弹出验证框。
 * B249：连接放在 terminal/sessions 里，离开页面时可以留在后台，回来接着用。
 */
export default function TerminalTab({ host }: { host: HostDetail }) {
  const t = useT();
  const location = useLocation();
  const slot = useRef<HTMLDivElement>(null);
  const info = useTerminals((s) => s.sessions[host.id]) as TermInfo | undefined;
  const leave = usePreferencesStore((s) => s.terminalLeave);
  const setLeave = usePreferencesStore((s) => s.setTerminalLeave);
  const [ended, setEnded] = useState(false);
  const [error, setError] = useState("");
  const status = info?.status ?? (ended ? "closed" : "idle");
  const path = terminalPath(location.pathname, host.id);
  const pathRef = useRef(path);
  pathRef.current = path;
  // 离开时怎么处理：弹窗里选的，没选过就按偏好
  const decision = useRef<"keep" | "close" | null>(null);

  const connect = useCallback(async () => {
    setError("");
    try {
      await withElevation(ensureElevated);
    } catch (e) {
      setError(errorMessage(e));
      return;
    }
    if (!slot.current) return;
    openSession({
      hostId: host.id,
      hostName: host.name,
      path: pathRef.current,
      slot: slot.current,
      closedLine: (reason) =>
        `\r\n\x1b[90m[${t("Connection closed")}${reason ? `: ${reason}` : ""}]\x1b[0m\r\n`,
    });
  }, [host.id, host.name, t]);

  const send = useCallback(
    (text: string) => sendInput(host.id, text),
    [host.id],
  );

  // 有留在后台的会话就接着用。离开时按选择留在后台或断开。
  useLayoutEffect(() => {
    const id = host.id;
    if (slot.current) showSession(id, slot.current);
    return () => {
      const pref = usePreferencesStore.getState().terminalLeave;
      const choice = decision.current ?? (pref === "close" ? "close" : "keep");
      decision.current = null;
      leaveSession(id, choice, pathRef.current, (evicted) =>
        toast(evictedMessage(t, evicted.hostName)),
      );
    };
    // t 只用来写提示，换语言不用重新挂
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [host.id]);

  // 跟着容器大小调整终端。
  useEffect(() => {
    const node = slot.current;
    if (!node) return;
    const ro = new ResizeObserver(() => fitSession(host.id));
    ro.observe(node);
    return () => ro.disconnect();
  }, [host.id]);

  // 连着的时候离开先问（偏好是“每次问”时）
  const blocker = useBlocker(
    ({ currentLocation, nextLocation }) =>
      status === "open" &&
      leave === "ask" &&
      currentLocation.pathname !== nextLocation.pathname,
  );
  const choose = (choice: "keep" | "close", remember: boolean) => {
    decision.current = choice;
    if (remember) setLeave(choice);
    blocker.proceed?.();
  };

  return (
    <div className="xc-card servers-terminal-card">
      <div className="xc-card-head">
        <h2>{t("Terminal")}</h2>
        <div className="xc-row">
          <span
            className={`xc-badge ${status === "open" ? "ok" : status === "connecting" ? "info" : ""}`}
          >
            {t(
              status === "open"
                ? "Connected"
                : status === "connecting"
                  ? "Connecting"
                  : status === "closed"
                    ? "Disconnected"
                    : "Not connected",
            )}
          </span>
          {status === "open" || status === "connecting" ? (
            <button
              className="xc-btn small"
              onClick={() => {
                disconnect(host.id);
                setEnded(true);
              }}
            >
              {t("Disconnect")}
            </button>
          ) : (
            <button
              className="xc-btn small primary"
              onClick={connect}
              disabled={!host.online}
            >
              {status === "closed" ? (
                <RotateCw size={13} />
              ) : (
                <Plug size={13} />
              )}{" "}
              {t(status === "closed" ? "Reconnect" : "Connect")}
            </button>
          )}
        </div>
      </div>
      {error && <p className="xc-error-text">{error}</p>}
      {!host.online && (
        <p className="xc-muted">{t("The machine is offline.")}</p>
      )}
      {status === "idle" && (
        <p className="xc-muted">
          {t("Opening a terminal needs your verification code.")}
        </p>
      )}
      <div className="servers-terminal-body">
        <div
          className={`servers-terminal${status === "idle" ? " idle" : ""}`}
          ref={slot}
        />
        <TerminalScripts
          host={host}
          connected={status === "open"}
          send={send}
        />
      </div>
      <LeaveDialog
        open={blocker.state === "blocked"}
        onChoose={choose}
        onCancel={() => blocker.reset?.()}
      />
    </div>
  );
}
