import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useParams } from "react-router";
import {
  CornerDownLeft,
  Delete,
  MonitorX,
  Send,
  ZoomIn,
  ZoomOut,
} from "lucide-react";
import { errorMessage, wsUrl } from "../../api/client";
import { withElevation } from "../../auth/elevation";
import { ErrorState, Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { usePageCrumb } from "../../stores/page-title";
import {
  ensureElevated,
  useAnswerWatcher,
  useRunWatcher,
  useWatcher,
} from "./api";
import { toPage } from "./lib";
import { PendingCard } from "./WatcherPage";
import "./i18n";
import "./watchers.css";

type State = "connecting" | "open" | "closed" | "error";

/**
 * 人工接管（B205、B206）：看服务器浏览器的画面，在上面点、拖、输入，
 * 用来过验证码、扫码、手动登录。弄好了点“我弄好了”。
 */
export default function LivePage() {
  const t = useT();
  const id = Number(useParams().watcherId);
  const watcher = useWatcher(id);
  usePageCrumb(t("Take over"), [
    { label: watcher.data?.name ?? "", to: `/monitoring/watchers/${id}` },
  ]);
  const answer = useAnswerWatcher();
  const run = useRunWatcher();
  const live = !!watcher.data?.pending?.live;
  const [nonce, setNonce] = useState(0);

  if (watcher.isPending) return <Loading />;
  if (watcher.isError)
    return (
      <div className="xc-page">
        <ErrorState error={watcher.error} onRetry={() => watcher.refetch()} />
      </div>
    );
  const w = watcher.data;
  return (
    <div className="xc-page watchers-live-page">
      {w.pending && <PendingCard w={w} onLivePage />}
      {live ? (
        <>
          <Screen key={nonce} id={id} onClosed={() => watcher.refetch()} />
          {w.pending?.kind !== "approve" && (
            <div className="watchers-live-done">
              <button
                className="xc-btn primary"
                disabled={answer.isPending}
                onClick={() =>
                  answer.mutate(
                    { id, answer: "done" },
                    {
                      onSuccess: () => toast("好的，接着往下走"),
                      onError: (e) =>
                        toast({ message: errorMessage(e), tone: "error" }),
                    },
                  )
                }
              >
                {t("I'm done")}
              </button>
              <button
                className="xc-btn ghost"
                onClick={() => setNonce((n) => n + 1)}
              >
                {t("Reconnect")}
              </button>
            </div>
          )}
        </>
      ) : (
        <div className="xc-card watchers-live-empty">
          <MonitorX size={26} />
          <p>
            现在没有打开的浏览器。要先登录一次（比如扫码登录），点下面的按钮打开。
          </p>
          <div className="xc-row">
            <Link className="xc-btn" to={`/monitoring/watchers/${id}`}>
              {t("Back to the watcher")}
            </Link>
            <button
              className="xc-btn primary"
              disabled={run.isPending}
              onClick={() =>
                run.mutate(
                  { id, kind: "manual" },
                  {
                    onError: (e) =>
                      toast({ message: errorMessage(e), tone: "error" }),
                  },
                )
              }
            >
              {t("Open the browser")}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

function Screen({ id, onClosed }: { id: number; onClosed: () => void }) {
  const t = useT();
  const img = useRef<HTMLImageElement | null>(null);
  const socket = useRef<WebSocket | null>(null);
  const size = useRef({ width: 1280, height: 900 });
  const dragging = useRef(false);
  const lastUrl = useRef("");
  const [state, setState] = useState<State>("connecting");
  const [error, setError] = useState("");
  const [text, setText] = useState("");
  // 手机上画面太小，放大后可以左右拖着看
  const [zoom, setZoom] = useState(false);
  // 父组件每次渲染都给新函数，放进 ref，免得重连
  const closedRef = useRef(onClosed);
  closedRef.current = onClosed;

  const send = useCallback((msg: Record<string, unknown>) => {
    const ws = socket.current;
    if (ws && ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(msg));
  }, []);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        await withElevation(ensureElevated);
      } catch (e) {
        if (!cancelled) {
          setError(errorMessage(e));
          setState("error");
        }
        return;
      }
      if (cancelled) return;
      const ws = new WebSocket(wsUrl(`/watchers/${id}/live`));
      ws.binaryType = "blob";
      socket.current = ws;
      ws.onopen = () => setState("open");
      ws.onmessage = (ev) => {
        if (typeof ev.data === "string") {
          try {
            const msg = JSON.parse(ev.data) as {
              type: string;
              width?: number;
              height?: number;
            };
            if (msg.type === "frame" && msg.width && msg.height)
              size.current = { width: msg.width, height: msg.height };
            if (msg.type === "closed") {
              setState("closed");
              closedRef.current();
            }
          } catch {
            // 不是 JSON 就不管
          }
          return;
        }
        const url = URL.createObjectURL(ev.data as Blob);
        if (img.current) img.current.src = url;
        if (lastUrl.current) URL.revokeObjectURL(lastUrl.current);
        lastUrl.current = url;
      };
      ws.onclose = () => {
        if (!cancelled) setState((s) => (s === "error" ? s : "closed"));
      };
      ws.onerror = () => {
        if (!cancelled) {
          setError("连不上浏览器画面");
          setState("error");
        }
      };
    })();
    return () => {
      cancelled = true;
      socket.current?.close();
      socket.current = null;
      if (lastUrl.current) URL.revokeObjectURL(lastUrl.current);
    };
  }, [id]);

  const point = (e: React.PointerEvent) => {
    const rect = e.currentTarget.getBoundingClientRect();
    return toPage(e.clientX, e.clientY, rect, size.current);
  };

  const sendText = () => {
    if (!text) return;
    send({ type: "text", text });
    setText("");
  };

  return (
    <section className="xc-card watchers-live">
      {state === "connecting" && <Loading />}
      {state === "error" && (
        <p className="watchers-detail-note danger">{error}</p>
      )}
      {state === "closed" && (
        <p className="watchers-detail-note">浏览器已经关了。</p>
      )}
      <div className="watchers-screen-wrap">
        <div
          className={`watchers-screen${zoom ? " zoom" : ""}`}
          tabIndex={0}
          role="application"
          aria-label={t("Server browser")}
          onPointerDown={(e) => {
            e.currentTarget.focus();
            if (zoom) return; // 放大时拖动是滚动画面，点一下在 pointerup 里发
            e.currentTarget.setPointerCapture(e.pointerId);
            dragging.current = true;
            send({ type: "down", ...point(e) });
          }}
          onPointerMove={(e) => {
            if (dragging.current && !zoom) send({ type: "drag", ...point(e) });
          }}
          onPointerUp={(e) => {
            if (zoom) {
              send({ type: "click", ...point(e) });
              return;
            }
            dragging.current = false;
            send({ type: "up", ...point(e) });
          }}
          onWheel={(e) => {
            const rect = e.currentTarget.getBoundingClientRect();
            send({
              type: "wheel",
              ...toPage(e.clientX, e.clientY, rect, size.current),
              deltaY: e.deltaY,
            });
          }}
          onKeyDown={(e) => {
            const keys = [
              "Enter",
              "Tab",
              "Backspace",
              "Escape",
              "ArrowDown",
              "ArrowUp",
            ];
            if (keys.includes(e.key)) {
              e.preventDefault();
              send({ type: "key", key: e.key });
            } else if (e.key.length === 1 && !e.ctrlKey && !e.metaKey) {
              e.preventDefault();
              send({ type: "text", text: e.key });
            }
          }}
        >
          <img ref={img} alt="" draggable={false} />
        </div>
      </div>
      <form
        className="watchers-live-input"
        onSubmit={(e) => {
          e.preventDefault();
          sendText();
        }}
      >
        <input
          className="xc-input"
          value={text}
          placeholder="先点画面上的输入框，再在这里输入验证码"
          onChange={(e) => setText(e.target.value)}
        />
        <button
          className="xc-btn"
          type="submit"
          title={t("Send")}
          aria-label={t("Send")}
        >
          <Send size={15} />
        </button>
        <button
          className="xc-btn"
          type="button"
          title="Backspace"
          aria-label="Backspace"
          onClick={() => send({ type: "key", key: "Backspace" })}
        >
          <Delete size={15} />
        </button>
        <button
          className="xc-btn"
          type="button"
          title={zoom ? t("Zoom out") : t("Zoom in")}
          aria-label={zoom ? t("Zoom out") : t("Zoom in")}
          onClick={() => setZoom((z) => !z)}
        >
          {zoom ? <ZoomOut size={15} /> : <ZoomIn size={15} />}
        </button>
        <button
          className="xc-btn"
          type="button"
          title="Enter"
          aria-label="Enter"
          onClick={() => send({ type: "key", key: "Enter" })}
        >
          <CornerDownLeft size={15} />
        </button>
      </form>
    </section>
  );
}
