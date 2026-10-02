import { lazy, Suspense, useEffect, useState } from "react";
import Markdown from "../../../components/markdown/Markdown";
import { Loading } from "../../../components/ui/States";
import { Segmented } from "../../../components/ui/Toolbar";
import { useT } from "../../../contexts/LanguageContext";
import { formatBytes } from "../../../lib/time";
import FileIcon from "../components/FileIcon";
import { viewerKind, decodeUtf8 } from "../viewer/kind";

const CodeEditor = lazy(() => import("../viewer/CodeEditor"));

/** 文本预览最多读这么多，再多就只显示开头（B75）。 */
export const SHARE_TEXT_BYTES = 2 * 1024 * 1024;

type File = { name: string; mime?: string; size: number };

/**
 * 分享页的预览区（B75）。地址带 preview=true，不算下载次数。
 * 图片、视频、音频、PDF 用浏览器自己的播放器；文本和 Markdown 读前 2 MB 显示；
 * 别的类型只显示图标和“不能预览”。
 */
export default function SharePreview({
  file,
  src,
}: {
  file: File;
  src: string;
}) {
  const t = useT();
  const kind = viewerKind({ ...file, isDir: false });
  if (kind === "image")
    return (
      <div className="share-stage">
        <img className="share-preview" src={src} alt={file.name} />
      </div>
    );
  if (kind === "video")
    return (
      <div className="share-stage">
        <video className="share-preview" src={src} controls playsInline />
      </div>
    );
  if (kind === "audio")
    return (
      <div className="share-stage">
        <audio src={src} controls />
      </div>
    );
  if (kind === "pdf")
    return (
      <iframe className="share-preview share-pdf" src={src} title={file.name} />
    );
  if (kind === "text" || kind === "markdown" || kind === "log")
    return <ShareText file={file} src={src} markdown={kind === "markdown"} />;
  return (
    <div className="share-noview">
      <FileIcon item={{ ...file, isDir: false }} size={40} />
      <strong title={file.name}>{file.name}</strong>
      <small className="drive-muted">{formatBytes(file.size)}</small>
      <span className="drive-muted">
        {t("This file cannot be previewed in the browser")}
      </span>
    </div>
  );
}

type Loaded = { text: string; binary: boolean };

async function loadText(src: string): Promise<Loaded> {
  const r = await fetch(src, {
    headers: { Range: `bytes=0-${SHARE_TEXT_BYTES - 1}` },
  });
  if (!r.ok) {
    let message = `HTTP ${r.status}`;
    try {
      message = ((await r.json()) as { message?: string }).message ?? message;
    } catch {
      /* 不是 JSON 就用状态码 */
    }
    throw new Error(message);
  }
  let bytes = new Uint8Array(await r.arrayBuffer());
  // 服务端不支持 Range 时会回整个文件，自己截断。
  if (bytes.length > SHARE_TEXT_BYTES)
    bytes = bytes.subarray(0, SHARE_TEXT_BYTES);
  if (bytes.subarray(0, 8192).includes(0)) return { text: "", binary: true };
  // 截断处可能把一个字切成两半，去掉末尾不完整的字节再按 UTF-8 解。
  let text = decodeUtf8(bytes);
  for (let cut = 1; text == null && cut <= 3; cut++)
    text = decodeUtf8(bytes.subarray(0, bytes.length - cut));
  return { text: text ?? new TextDecoder("gbk").decode(bytes), binary: false };
}

function ShareText({
  file,
  src,
  markdown,
}: {
  file: File;
  src: string;
  markdown: boolean;
}) {
  const t = useT();
  const [loaded, setLoaded] = useState<Loaded | null>(null);
  const [error, setError] = useState("");
  const [source, setSource] = useState(false);
  useEffect(() => {
    let alive = true;
    setLoaded(null);
    setError("");
    loadText(src)
      .then((r) => alive && setLoaded(r))
      .catch(
        (e: unknown) =>
          alive && setError(e instanceof Error ? e.message : String(e)),
      );
    return () => {
      alive = false;
    };
  }, [src]);
  if (error) return <p className="share-error">{error}</p>;
  if (!loaded) return <Loading />;
  if (loaded.binary)
    return (
      <div className="share-noview">
        <FileIcon item={{ ...file, isDir: false }} size={40} />
        <span className="drive-muted">
          {t("This file cannot be previewed in the browser")}
        </span>
      </div>
    );
  const cut = file.size > SHARE_TEXT_BYTES;
  return (
    <div className="share-text">
      {markdown && (
        <div className="share-text-bar">
          <Segmented
            label={t("View")}
            value={source ? "source" : "rendered"}
            onChange={(v) => setSource(v === "source")}
            options={[
              { value: "rendered", label: t("Preview") },
              { value: "source", label: t("Source code") },
            ]}
          />
        </div>
      )}
      <div className="share-text-body">
        {markdown && !source ? (
          <div className="drive-markdown">
            <Markdown source={loaded.text} />
          </div>
        ) : (
          <Suspense fallback={<Loading />}>
            <CodeEditor
              name={file.name}
              text={loaded.text}
              editable={false}
              onChange={() => {}}
              onSave={() => {}}
            />
          </Suspense>
        )}
      </div>
      {cut && (
        <p className="share-text-cut">
          {t(
            "The file is too large. Only the beginning is shown. Download it to see everything.",
          )}
        </p>
      )}
    </div>
  );
}
