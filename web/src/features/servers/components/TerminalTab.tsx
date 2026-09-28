import { useCallback, useEffect, useRef, useState } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import { Plug, RotateCw } from "lucide-react";
import { errorMessage, wsUrl } from "../../../api/client";
import { withElevation } from "../../../auth/elevation";
import { useT } from "../../../contexts/LanguageContext";
import { ensureElevated, type HostDetail } from "../api";

type Status = "idle" | "connecting" | "open" | "closed";

function cssVar(name: string, fallback: string) {
  const v = getComputedStyle(document.documentElement)
    .getPropertyValue(name)
    .trim();
  return v || fallback;
}

/**
 * xterm.js 终端。二进制帧是终端数据，文本帧是窗口大小。
 * 打开前先确认 5 分钟内验证过，否则弹出验证框。
 */
export default function TerminalTab({ host }: { host: HostDetail }) {
  const t = useT();
  const el = useRef<HTMLDivElement>(null);
  const term = useRef<Terminal | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const socket = useRef<WebSocket | null>(null);
  const [status, setStatus] = useState<Status>("idle");
  const [error, setError] = useState("");

  const disconnect = useCallback(() => {
    socket.current?.close();
    socket.current = null;
    term.current?.dispose();
    term.current = null;
    fitRef.current = null;
  }, []);

  const connect = useCallback(async () => {
    setError("");
    try {
      await withElevation(ensureElevated);
    } catch (e) {
      setError(errorMessage(e));
      return;
    }
    disconnect();
    if (!el.current) return;
    setStatus("connecting");
    const xterm = new Terminal({
      cursorBlink: true,
      fontFamily: cssVar("--xc-mono", "monospace"),
      fontSize: 13,
      scrollback: 5000,
      theme: {
        background: cssVar("--servers-term-bg", "#0a0a0b"),
        foreground: cssVar("--servers-term-fg", "#ededf0"),
        cursor: cssVar("--xc-accent", "#cc7752"),
        selectionBackground: "#5c5c6480",
      },
    });
    const fit = new FitAddon();
    xterm.loadAddon(fit);
    xterm.open(el.current);
    fit.fit();
    term.current = xterm;
    fitRef.current = fit;

    const ws = new WebSocket(
      wsUrl(
        `/hosts/${encodeURIComponent(host.id)}/terminal?cols=${xterm.cols}&rows=${xterm.rows}`,
      ),
    );
    ws.binaryType = "arraybuffer";
    socket.current = ws;
    const encoder = new TextEncoder();
    ws.onopen = () => {
      setStatus("open");
      xterm.focus();
    };
    ws.onmessage = (event) => {
      if (event.data instanceof ArrayBuffer)
        xterm.write(new Uint8Array(event.data));
    };
    ws.onclose = (event) => {
      if (socket.current !== ws) return;
      setStatus("closed");
      const reason =
        event.reason && event.reason !== "terminal closed"
          ? `: ${event.reason}`
          : "";
      xterm.write(
        `\r\n\x1b[90m[${t("Connection closed")}${reason}]\x1b[0m\r\n`,
      );
    };
    xterm.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) ws.send(encoder.encode(data));
    });
    xterm.onBinary((data) => {
      if (ws.readyState !== WebSocket.OPEN) return;
      const bytes = Uint8Array.from(data, (c) => c.charCodeAt(0) & 0xff);
      ws.send(bytes);
    });
    xterm.onResize(({ cols, rows }) => {
      if (ws.readyState === WebSocket.OPEN)
        ws.send(JSON.stringify({ type: "resize", cols, rows }));
    });
  }, [host.id, disconnect, t]);

  // 跟着容器大小调整终端。
  useEffect(() => {
    const node = el.current;
    if (!node) return;
    const ro = new ResizeObserver(() => {
      try {
        fitRef.current?.fit();
      } catch {
        /* 容器还没显示出来 */
      }
    });
    ro.observe(node);
    return () => ro.disconnect();
  }, []);

  // 离开页面时关闭连接，代理端的 shell 随之退出。
  useEffect(() => disconnect, [disconnect]);

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
                disconnect();
                setStatus("closed");
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
      <div
        className={`servers-terminal${status === "idle" ? " idle" : ""}`}
        ref={el}
      />
    </div>
  );
}
