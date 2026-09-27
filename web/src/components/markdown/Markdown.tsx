import { Fragment, useMemo, type ReactNode } from "react";
import { Link } from "react-router";
import { parseMarkdown, type Block, type Inline } from "./mdparse";
import "./markdown.css";

interface Options {
  /** 传了就能在预览里勾选待办，参数是第几个待办（从 0 开始）。 */
  onToggleTask?: (index: number) => void;
  /** 点图片时调用，比如打开大图。 */
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
  if (blocks.length === 0 && empty) return <>{empty}</>;
  const ctx: Ctx = { task: 0, onToggleTask, onImageClick };
  return <div className={`xc-md ${className}`}>{renderBlocks(blocks, ctx)}</div>;
}

interface Ctx extends Options {
  task: number;
}

function renderBlocks(blocks: Block[], ctx: Ctx): ReactNode {
  return blocks.map((block, i) => <Fragment key={i}>{renderBlock(block, ctx)}</Fragment>);
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
      return (
        <pre>
          <code data-lang={block.lang || undefined}>{block.text}</code>
        </pre>
      );
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
            className={item.checked !== null ? `task${item.checked ? " done" : ""}` : undefined}
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
      case "image":
        return (
          <img
            key={i}
            src={node.src}
            alt={node.alt}
            loading="lazy"
            onClick={ctx.onImageClick ? () => ctx.onImageClick!(node.src, node.alt) : undefined}
          />
        );
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
