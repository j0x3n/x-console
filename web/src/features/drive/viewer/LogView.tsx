import { useEffect, useRef, useState } from "react";
import { wsUrl } from "../../../api/client";
import LogViewer, { type LogLine } from "../../../components/log/LogViewer";
import { ErrorState, Loading } from "../../../components/ui/States";
import Switch from "../../../components/ui/Switch";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import { formatBytes } from "../../../lib/time";
import { readContent, type DriveFollowFrame, type DriveItem } from "../api";

/** 读文件的 [start, end) 这一段，end 不传就读到结尾。 */
export type ReadRange = (start: number, end?: number) => Promise<Uint8Array>;

/** 云盘里的文件。 */
export default function LogView({ item }: { item: DriveItem }) {
  const read: ReadRange = (start, end) =>
    readContent(
      item.id,
      end !== undefined
        ? `bytes=${start}-${end - 1}`
        : start > 0
          ? `bytes=${start}-`
          : undefined,
    ).then((r) => r.bytes);
  return (
    <RangeLogView
      fileKey={`drive-${item.id}`}
      size={item.size}
      read={read}
      followPath={(offset) => `/drive/items/${item.id}/follow?offset=${offset}`}
    />
  );
}
import { appendLog, concatBytes, LOG_CHUNK_BYTES, splitChunk } from "./kind";

/**
 * 日志和大文本：先读最后 1 MB，滚到顶再往前读 1 MB。
 * 不一次读整个文件。
 * “实时”打开后通过 WebSocket 接收追加的内容，接到最后并自动滚动；
 * 文件被轮转（变小）时从头开始。
 */
export function RangeLogView({
  fileKey,
  size,
  read,
  followPath,
}: {
  /** 换文件时变，用来重新读。 */
  fileKey: string;
  size: number;
  read: ReadRange;
  /** 实时模式的 WebSocket 路径（不含 /api/v1）。 */
  followPath: (offset: number) => string;
}) {
  const t = useT();
  const [lines, setLines] = useState<LogLine[] | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [loading, setLoading] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const [live, setLive] = useState(false);
  // 还没读的部分从 0 到 start；head 是上一段开头不完整的那一行。
  // end 是已经读到的位置，实时模式从这里接着收。
  const state = useRef<{
    start: number;
    head: Uint8Array;
    nextId: number;
    end: number;
    maxId: number;
    openLast: boolean;
  }>({
    start: 0,
    head: new Uint8Array(0),
    nextId: 0,
    end: 0,
    maxId: 0,
    openLast: false,
  });

  useEffect(() => {
    let alive = true;
    setLines(null);
    setError(null);
    setLive(false);
    const start = Math.max(0, size - LOG_CHUNK_BYTES);
    read(start)
      .then((bytes) => {
        if (!alive) return;
        const { head, lines } = splitChunk(bytes, start === 0);
        state.current = {
          start,
          head,
          nextId: 0,
          end: start + bytes.length,
          maxId: lines.length,
          openLast: bytes.length > 0 && bytes[bytes.length - 1] !== 10,
        };
        // 先读到的是最后面的行，编号从 0 往上；更早的行用负数，保证递增。
        setLines(lines.map((text, i) => ({ id: i, text })));
      })
      .catch((e) => alive && setError(e));
    return () => {
      alive = false;
    };
    // read 每次渲染都是新函数，只在换文件时重新读。
  }, [fileKey, size, attempt]);

  // 实时模式。
  useEffect(() => {
    if (!live) return;
    const ws = new WebSocket(wsUrl(followPath(state.current.end)));
    let opened = false;
    ws.onopen = () => {
      opened = true;
    };
    ws.onmessage = (event) => {
      if (typeof event.data !== "string") return;
      let frame: DriveFollowFrame;
      try {
        frame = JSON.parse(event.data) as DriveFollowFrame;
      } catch {
        return;
      }
      const s = state.current;
      if (frame.type === "reset") {
        // 日志被轮转了：前面的内容不再对应这个文件，从头开始。
        s.start = 0;
        s.head = new Uint8Array(0);
        s.end = 0;
        s.openLast = false;
        setLines([]);
        return;
      }
      const data = frame.data ?? "";
      s.end = (frame.offset ?? s.end) + new TextEncoder().encode(data).length;
      setLines((cur) => {
        const r = appendLog(cur ?? [], s.openLast, data, s.maxId);
        s.openLast = r.openLast;
        s.maxId = r.nextId;
        return r.lines;
      });
    };
    ws.onclose = (event) => {
      if (event.code === 1000) return;
      setLive(false);
      toast({
        message: opened
          ? t("Live mode disconnected")
          : "实时模式连不上，服务端可能还没上线这个功能。",
        tone: "error",
      });
    };
    return () => ws.close(1000);
  }, [live, fileKey]);

  const loadOlder = () => {
    const s = state.current;
    if (loading || s.start === 0) return;
    const from = Math.max(0, s.start - LOG_CHUNK_BYTES);
    setLoading(true);
    read(from, s.start)
      .then((bytes) => {
        const all = concatBytes(bytes, s.head);
        const { head, lines } = splitChunk(all, from === 0);
        const first = s.nextId - lines.length;
        Object.assign(state.current, { start: from, head, nextId: first });
        setLines((cur) => [
          ...lines.map((text, i) => ({ id: first + i, text })),
          ...(cur ?? []),
        ]);
      })
      .catch(setError)
      .finally(() => setLoading(false));
  };

  if (error)
    return (
      <div className="drive-viewer-center">
        <ErrorState error={error} onRetry={() => setAttempt((n) => n + 1)} />
      </div>
    );
  if (!lines) return <Loading />;
  const start = state.current.start;
  return (
    <div className="drive-log">
      <LogViewer
        lines={lines}
        height="100%"
        onReachTop={start > 0 ? loadOlder : undefined}
        loadingOlder={loading}
        toolbarEnd={
          <>
            {start > 0 && (
              <small className="drive-muted drive-log-more">
                前面还有 {formatBytes(start)}，往上滚继续读
              </small>
            )}
            <span className="drive-log-live">
              <Switch checked={live} onChange={setLive} label={t("Live")} />
              <small>{t("Live")}</small>
            </span>
          </>
        }
        empty={<p className="drive-muted">文件是空的。</p>}
      />
    </div>
  );
}
