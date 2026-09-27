import { Fragment, useMemo, type ReactNode } from "react";
import { Link } from "react-router";
import { parseMarkdown, type Block, type Inline } from "./mdparse";

/** 渲染 Markdown。站内路径用路由跳转，外部链接在新标签页打开。 */
export default function Markdown({
  source,
  empty,
  className = "",
}: {
  source: string;
  empty?: ReactNode;
  className?: string;
}) {
  const blocks = useMemo(() => parseMarkdown(source), [source]);
  if (blocks.length === 0 && empty) return <>{empty}</>;
  return (
    <div className={`projects-md ${className}`}>{renderBlocks(blocks)}</div>
  );
}

function renderBlocks(blocks: Block[]): ReactNode {
  return blocks.map((block, i) => (
    <Fragment key={i}>{renderBlock(block)}</Fragment>
  ));
}

function renderBlock(block: Block): ReactNode {
  switch (block.type) {
    case "heading": {
      const Tag = `h${Math.min(block.level + 1, 6)}` as "h2";
      return <Tag>{renderInline(block.children)}</Tag>;
    }
    case "paragraph":
      return <p>{renderInline(block.children)}</p>;
    case "code":
      return (
        <pre>
          <code data-lang={block.lang || undefined}>{block.text}</code>
        </pre>
      );
    case "quote":
      return <blockquote>{renderBlocks(block.blocks)}</blockquote>;
    case "hr":
      return <hr />;
    case "list": {
      const items = block.items.map((item, i) => (
        <li key={i} className={item.checked !== null ? "task" : undefined}>
          {item.checked !== null && (
            <input
              type="checkbox"
              checked={item.checked}
              readOnly
              tabIndex={-1}
            />
          )}
          {item.blocks.length === 1 && item.blocks[0].type === "paragraph"
            ? renderInline(item.blocks[0].children)
            : renderBlocks(item.blocks)}
        </li>
      ));
      return block.ordered ? (
        <ol start={block.start !== 1 ? block.start : undefined}>{items}</ol>
      ) : (
        <ul>{items}</ul>
      );
    }
  }
}

function renderInline(nodes: Inline[]): ReactNode {
  return nodes.map((node, i) => {
    switch (node.type) {
      case "text":
        return <Fragment key={i}>{node.text}</Fragment>;
      case "br":
        return <br key={i} />;
      case "code":
        return <code key={i}>{node.text}</code>;
      case "strong":
        return <strong key={i}>{renderInline(node.children)}</strong>;
      case "em":
        return <em key={i}>{renderInline(node.children)}</em>;
      case "del":
        return <del key={i}>{renderInline(node.children)}</del>;
      case "link":
        return node.href.startsWith("/") ? (
          <Link key={i} to={node.href}>
            {renderInline(node.children)}
          </Link>
        ) : (
          <a key={i} href={node.href} target="_blank" rel="noreferrer noopener">
            {renderInline(node.children)}
          </a>
        );
    }
  });
}
