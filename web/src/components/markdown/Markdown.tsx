import { Fragment, useMemo, useState, type ReactNode } from "react";
import { Link } from "react-router";
import {
  AlertTriangle,
  ChevronDown,
  ChevronRight,
  Copy,
  EyeOff,
  Info,
  Lightbulb,
  Link2,
  OctagonAlert,
} from "lucide-react";
import {
  mediaKind,
  parseMarkdown,
  type Block,
  type ContainerKind,
  type Inline,
} from "./mdparse";
import "./markdown.css";
import { useT } from "../../contexts/LanguageContext";
import ImageLightbox, { type LightboxImage } from "../ui/ImageLightbox";
import { isUploadedFile, thumbnailSrc } from "./upload";

interface Options {
  /** 传了就能在预览里勾选待办，参数是第几个待办（从 0 开始）。 */
  onToggleTask?: (index: number) => void;
  /** 点图片时调用。不传时在页内打开大图（B54）。 */
  onImageClick?: (src: string, alt: string) => void;
}

/** 渲染 Markdown。站内路径用路由跳转，外部链接在新标签页打开。 */
export default function Markdown({
  source,
  empty,
  className = "",
  onToggleTask,
  onImageClick,
}: {
  source: string;
  empty?: ReactNode;
  className?: string;
} & Options) {
  const blocks = useMemo(() => parseMarkdown(source), [source]);
  const images = useMemo(() => collectImages(blocks), [blocks]);
  const [open, setOpen] = useState<number | null>(null);
  if (blocks.length === 0 && empty) return <>{empty}</>;
  const ctx: Ctx = {
    task: 0,
    onToggleTask,
    onImageClick:
      onImageClick ??
      ((src) => {
        const i = images.findIndex((x) => x.src === src);
        setOpen(i < 0 ? 0 : i);
      }),
  };
  return (
    <div className={`xc-md ${className}`}>
      {renderBlocks(blocks, ctx)}
      {open !== null && (
        <ImageLightbox
          images={images}
          start={open}
          onClose={() => setOpen(null)}
        />
      )}
    </div>
  );
}

interface Ctx extends Options {
  task: number;
}

function renderBlocks(blocks: Block[], ctx: Ctx): ReactNode {
  return blocks.map((block, i) => (
    <Fragment key={i}>{renderBlock(block, ctx)}</Fragment>
  ));
}

function renderBlock(block: Block, ctx: Ctx): ReactNode {
  switch (block.type) {
    case "heading": {
      const Tag = `h${Math.min(block.level + 1, 6)}` as "h2";
      return <Tag>{renderInline(block.children, ctx)}</Tag>;
    }
    case "paragraph":
      return <p>{renderInline(block.children, ctx)}</p>;
    case "code":
      return <CodeBlock lang={block.lang} text={block.text} />;
    case "table":
      return (
        <div className="xc-md-table">
          <table>
            <thead>
              <tr>
                {block.header.map((cell, k) => (
                  <th
                    key={k}
                    style={{ textAlign: block.align[k] ?? undefined }}
                  >
                    {renderInline(cell, ctx)}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {block.rows.map((row, r) => (
                <tr key={r}>
                  {row.map((cell, k) => (
                    <td
                      key={k}
                      style={{ textAlign: block.align[k] ?? undefined }}
                    >
                      {renderInline(cell, ctx)}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      );
    case "container":
      return block.kind === "hidden" ? (
        <HiddenBlock title={block.title}>
          {renderBlocks(block.blocks, ctx)}
        </HiddenBlock>
      ) : (
        <Callout kind={block.kind} title={block.title}>
          {renderBlocks(block.blocks, ctx)}
        </Callout>
      );
    case "linkcard":
      return <LinkCard href={block.href} />;
    case "quote":
      return <blockquote>{renderBlocks(block.blocks, ctx)}</blockquote>;
    case "hr":
      return <hr />;
    case "list": {
      const items = block.items.map((item, i) => {
        // 先编号再渲染子节点，顺序和源码里出现的顺序一致。
        const index = item.checked !== null ? ctx.task++ : -1;
        const toggle = ctx.onToggleTask;
        const content =
          item.blocks.length === 1 && item.blocks[0].type === "paragraph" ? (
            <span>{renderInline(item.blocks[0].children, ctx)}</span>
          ) : (
            renderBlocks(item.blocks, ctx)
          );
        return (
          <li
            key={i}
            className={
              item.checked !== null
                ? `task${item.checked ? " done" : ""}`
                : undefined
            }
          >
            {item.checked !== null && (
              <input
                type="checkbox"
                checked={item.checked}
                readOnly={!toggle}
                tabIndex={toggle ? 0 : -1}
                onChange={toggle ? () => toggle(index) : undefined}
              />
            )}
            {content}
          </li>
        );
      });
      return block.ordered ? (
        <ol start={block.start !== 1 ? block.start : undefined}>{items}</ol>
      ) : (
        <ul>{items}</ul>
      );
    }
  }
}

function renderInline(nodes: Inline[], ctx: Ctx): ReactNode {
  return nodes.map((node, i) => {
    switch (node.type) {
      case "text":
        return <Fragment key={i}>{node.text}</Fragment>;
      case "br":
        return <br key={i} />;
      case "code":
        return <code key={i}>{node.text}</code>;
      case "strong":
        return <strong key={i}>{renderInline(node.children, ctx)}</strong>;
      case "em":
        return <em key={i}>{renderInline(node.children, ctx)}</em>;
      case "del":
        return <del key={i}>{renderInline(node.children, ctx)}</del>;
      case "mark":
        return <mark key={i}>{renderInline(node.children, ctx)}</mark>;
      case "image": {
        // 视频、音频用图片的写法，按后缀显示成播放器（B74）
        const media = mediaKind(node.src, node.alt);
        if (media === "video")
          return (
            <video
              key={i}
              className="xc-md-video"
              src={node.src}
              controls
              playsInline
              preload="metadata"
              title={node.alt}
            />
          );
        if (media === "audio")
          return (
            <audio
              key={i}
              className="xc-md-audio"
              src={node.src}
              controls
              preload="metadata"
              title={node.alt}
            />
          );
        // 公共上传的图片显示缩略图（B36），点开在页内看原图（B54）。
        return (
          <img
            key={i}
            src={isUploadedFile(node.src) ? thumbnailSrc(node.src) : node.src}
            alt={node.alt}
            loading="lazy"
            className="xc-md-zoom"
            onClick={(e) => {
              e.preventDefault();
              e.stopPropagation();
              ctx.onImageClick?.(node.src, node.alt);
            }}
          />
        );
      }
      case "link":
        return node.href.startsWith("/") && !node.href.startsWith("/api/") ? (
          <Link key={i} to={node.href}>
            {renderInline(node.children, ctx)}
          </Link>
        ) : (
          <a key={i} href={node.href} target="_blank" rel="noreferrer noopener">
            {renderInline(node.children, ctx)}
          </a>
        );
    }
  });
}

/** 按出现顺序取出全部图片，大图层里左右切换用。 */
function collectImages(blocks: Block[]): LightboxImage[] {
  const out: LightboxImage[] = [];
  const seen = new Set<string>();
  const walk = (value: unknown) => {
    if (Array.isArray(value)) return value.forEach(walk);
    if (!value || typeof value !== "object") return;
    const node = value as { type?: string; src?: string; alt?: string };
    if (node.type === "image" && node.src && !seen.has(node.src)) {
      seen.add(node.src);
      out.push({ src: node.src, alt: node.alt });
    }
    Object.values(value).forEach(walk);
  };
  walk(blocks);
  return out;
}

/** 超过这么多行默认折叠（B74）。 */
const FOLD_LINES = 20;
const FOLDED_LINES = 12;

/**
 * 代码块（B74）：顶部一条语言、行数、折叠、复制。
 * 顶部这条在代码块露在屏幕里时一直贴着可见区域的顶部，长代码往下滚也能点复制。
 */
function CodeBlock({ lang, text }: { lang: string; text: string }) {
  const t = useT();
  const lines = text.split("\n").length;
  const long = lines > FOLD_LINES;
  const [folded, setFolded] = useState(long);
  const [collapsed, setCollapsed] = useState(false);
  const [copied, setCopied] = useState(false);
  const shown = folded
    ? text.split("\n").slice(0, FOLDED_LINES).join("\n")
    : text;
  const copy = () =>
    navigator.clipboard?.writeText(text).then(() => {
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    });
  return (
    <div className={`xc-md-code${collapsed ? " collapsed" : ""}`}>
      <div className="xc-md-code-bar">
        <span>{lang || "text"}</span>
        <span>{t("{n} lines").replace("{n}", String(lines))}</span>
        <span className="xc-md-code-gap" />
        <button
          type="button"
          onClick={() => setCollapsed((v) => !v)}
          aria-expanded={!collapsed}
          title={collapsed ? t("Unfold") : t("Fold")}
        >
          {collapsed ? <ChevronRight size={13} /> : <ChevronDown size={13} />}
          {collapsed ? t("Unfold") : t("Fold")}
        </button>
        <button type="button" onClick={() => void copy()} title={t("Copy")}>
          <Copy size={13} />
          {copied ? t("Copied") : t("Copy")}
        </button>
      </div>
      {!collapsed && (
        <pre>
          <code data-lang={lang || undefined}>{shown}</code>
        </pre>
      )}
      {!collapsed && folded && (
        <button
          type="button"
          className="xc-md-code-more"
          onClick={() => setFolded(false)}
        >
          {t("Show all {n} lines").replace("{n}", String(lines))}
        </button>
      )}
    </div>
  );
}

/** 隐藏块（B74）：默认折叠，点开才显示。 */
function HiddenBlock({
  title,
  children,
}: {
  title: string;
  children: ReactNode;
}) {
  const t = useT();
  const [open, setOpen] = useState(false);
  return (
    <div className={`xc-md-hidden${open ? " open" : ""}`}>
      <button
        type="button"
        aria-expanded={open}
        onClick={(e) => {
          e.stopPropagation();
          setOpen((v) => !v);
        }}
        onDoubleClick={(e) => e.stopPropagation()}
      >
        <EyeOff size={14} />
        <span>
          {open
            ? t("Shown: {title}. Click to hide.").replace(
                "{title}",
                title || t("Hidden content"),
              )
            : t("Hidden: {title}. Click to show.").replace(
                "{title}",
                title || t("Hidden content"),
              )}
        </span>
      </button>
      {open && <div className="xc-md-hidden-body">{children}</div>}
    </div>
  );
}

const CALLOUT_ICONS: Record<Exclude<ContainerKind, "hidden">, ReactNode> = {
  note: <Info size={15} />,
  tip: <Lightbulb size={15} />,
  warn: <AlertTriangle size={15} />,
  danger: <OctagonAlert size={15} />,
};
const CALLOUT_TITLES: Record<Exclude<ContainerKind, "hidden">, string> = {
  note: "Callout note",
  tip: "Callout tip",
  warn: "Callout warning",
  danger: "Callout danger",
};

/** 提示块（B74）：浅色底和左边色条，图标随类型。 */
function Callout({
  kind,
  title,
  children,
}: {
  kind: Exclude<ContainerKind, "hidden">;
  title: string;
  children: ReactNode;
}) {
  const t = useT();
  return (
    <div className={`xc-md-callout ${kind}`}>
      <div className="xc-md-callout-title">
        {CALLOUT_ICONS[kind]}
        <span>{title || t(CALLOUT_TITLES[kind])}</span>
      </div>
      <div className="xc-md-callout-body">{children}</div>
    </div>
  );
}

/** 一段里只有一个网址时的链接卡片（B74）：域名和网址。不去抓网页。 */
function LinkCard({ href }: { href: string }) {
  let host = href;
  try {
    host = new URL(href).hostname;
  } catch {
    /* 解析不了就显示原文 */
  }
  return (
    <a
      className="xc-md-linkcard"
      href={href}
      target="_blank"
      rel="noreferrer noopener"
    >
      <span className="xc-md-linkcard-icon">
        <Link2 size={16} />
      </span>
      <span className="xc-md-linkcard-text">
        <strong>{host}</strong>
        <small>{href}</small>
      </span>
    </a>
  );
}
