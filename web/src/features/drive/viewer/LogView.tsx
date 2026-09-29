import { useEffect, useRef, useState } from "react";
import LogViewer, { type LogLine } from "../../../components/log/LogViewer";
import { ErrorState, Loading } from "../../../components/ui/States";
import { formatBytes } from "../../../lib/time";
import { readContent, type DriveItem } from "../api";
import { concatBytes, LOG_CHUNK_BYTES, splitChunk } from "./kind";

/**
 * 日志和大文本：先读最后 1 MB，滚到顶再往前读 1 MB。
 * 不一次读整个文件。
 */
export default function LogView({ item }: { item: DriveItem }) {
  const [lines, setLines] = useState<LogLine[] | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [loading, setLoading] = useState(false);
  const [attempt, setAttempt] = useState(0);
  // 还没读的部分从 0 到 start；head 是上一段开头不完整的那一行。
  const state = useRef<{ start: number; head: Uint8Array; nextId: number }>({
    start: 0,
    head: new Uint8Array(0),
    nextId: 0,
  });

  useEffect(() => {
    let alive = true;
    setLines(null);
    setError(null);
    const start = Math.max(0, item.size - LOG_CHUNK_BYTES);
    readContent(item.id, start > 0 ? `bytes=${start}-` : undefined)
      .then(({ bytes }) => {
        if (!alive) return;
        const { head, lines } = splitChunk(bytes, start === 0);
        state.current = { start, head, nextId: 0 };
        // 先读到的是最后面的行，编号从 0 往上；更早的行用负数，保证递增。
        setLines(lines.map((text, i) => ({ id: i, text })));
      })
      .catch((e) => alive && setError(e));
    return () => {
      alive = false;
    };
  }, [item.id, item.size, attempt]);

  const loadOlder = () => {
    const s = state.current;
    if (loading || s.start === 0) return;
    const from = Math.max(0, s.start - LOG_CHUNK_BYTES);
    setLoading(true);
    readContent(item.id, `bytes=${from}-${s.start - 1}`)
      .then(({ bytes }) => {
        const all = concatBytes(bytes, s.head);
        const { head, lines } = splitChunk(all, from === 0);
        const first = s.nextId - lines.length;
        state.current = { start: from, head, nextId: first };
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
          start > 0 && (
            <small className="drive-muted">
              前面还有 {formatBytes(start)}，往上滚继续读
            </small>
          )
        }
        empty={<p className="drive-muted">文件是空的。</p>}
      />
    </div>
  );
}
