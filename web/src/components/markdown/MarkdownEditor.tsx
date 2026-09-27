import { useLayoutEffect, useRef, useState, type ReactNode } from "react";
import {
  Bold,
  Code,
  Eye,
  Heading2,
  Italic,
  Link2,
  List,
  ListChecks,
  ListOrdered,
  Minus,
  Pencil,
  Quote,
} from "lucide-react";
import { useT } from "../../contexts/LanguageContext";
import Markdown from "./Markdown";
import { toggleTask } from "./mdparse";
import { insertBlock, prefixLines, wrapSelection, type Edit } from "./edit";

interface MarkdownEditorProps {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  /** 读屏用的名称，比如“描述” */
  label: string;
  /** 最少几行高，内容多了会自己长高 */
  minRows?: number;
  autoFocus?: boolean;
  /** Ctrl/⌘ + Enter 时调用，比如发评论 */
  onSubmit?: () => void;
  /** 工具条右边额外的东西 */
  extra?: ReactNode;
}

/**
 * 和笔记一样的 Markdown 编辑框：上面一排格式按钮，可以切到预览。
 * 项目描述、Issue、评论、日程备注、提醒备注这类长文字都用它。
 */
export default function MarkdownEditor({
  value,
  onChange,
  placeholder,
  label,
  minRows = 4,
  autoFocus,
  onSubmit,
  extra,
}: MarkdownEditorProps) {
  const t = useT();
  const ref = useRef<HTMLTextAreaElement>(null);
  const [preview, setPreview] = useState(false);
  const valueRef = useRef(value);
  valueRef.current = value;

  // 随内容长高
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el || preview) return;
    el.style.height = "auto";
    el.style.height = `${el.scrollHeight + 2}px`;
  }, [value, preview]);

  const apply = (fn: (text: string, start: number, end: number) => Edit) => {
    const el = ref.current;
    const text = valueRef.current;
    const start = el?.selectionStart ?? text.length;
    const end = el?.selectionEnd ?? text.length;
    const out = fn(text, start, end);
    onChange(out.text);
    setPreview(false);
    requestAnimationFrame(() => {
      const target = ref.current;
      if (!target) return;
      target.focus();
      target.setSelectionRange(out.start, out.end);
    });
  };

  const tools: {
    key: string;
    icon: ReactNode;
    label: string;
    run: () => void;
  }[] = [
    {
      key: "h",
      icon: <Heading2 size={15} />,
      label: t("Heading"),
      run: () => apply((x, s, e) => prefixLines(x, s, e, "## ")),
    },
    {
      key: "b",
      icon: <Bold size={15} />,
      label: t("Bold"),
      run: () =>
        apply((x, s, e) => wrapSelection(x, s, e, "**", "**", t("bold text"))),
    },
    {
      key: "i",
      icon: <Italic size={15} />,
      label: t("Italic"),
      run: () =>
        apply((x, s, e) => wrapSelection(x, s, e, "*", "*", t("italic text"))),
    },
    {
      key: "ul",
      icon: <List size={15} />,
      label: t("Bulleted list"),
      run: () => apply((x, s, e) => prefixLines(x, s, e, "- ")),
    },
    {
      key: "ol",
      icon: <ListOrdered size={15} />,
      label: t("Numbered list"),
      run: () => apply((x, s, e) => prefixLines(x, s, e, "", true)),
    },
    {
      key: "task",
      icon: <ListChecks size={15} />,
      label: t("Checklist"),
      run: () => apply((x, s, e) => prefixLines(x, s, e, "- [ ] ")),
    },
    {
      key: "quote",
      icon: <Quote size={15} />,
      label: t("Quote"),
      run: () => apply((x, s, e) => prefixLines(x, s, e, "> ")),
    },
    {
      key: "code",
      icon: <Code size={15} />,
      label: t("Code block"),
      run: () =>
        apply((x, s, e) =>
          x.slice(s, e).includes("\n") || s === e
            ? insertBlock(x, s, e, "```\n" + (x.slice(s, e) || "") + "\n```")
            : wrapSelection(x, s, e, "`"),
        ),
    },
    {
      key: "link",
      icon: <Link2 size={15} />,
      label: t("Link"),
      run: () =>
        apply((x, s, e) =>
          wrapSelection(x, s, e, "[", "](https://)", t("link text")),
        ),
    },
    {
      key: "hr",
      icon: <Minus size={15} />,
      label: t("Divider"),
      run: () => apply((x, s, e) => insertBlock(x, s, e, "---")),
    },
  ];

  return (
    <div className={`xc-mde${preview ? " is-preview" : ""}`}>
      <div
        className="xc-mde-toolbar"
        role="toolbar"
        aria-label={t("Formatting")}
      >
        {tools.map((tool) => (
          <button
            key={tool.key}
            type="button"
            title={tool.label}
            aria-label={tool.label}
            onMouseDown={(e) => e.preventDefault()}
            onClick={tool.run}
          >
            {tool.icon}
          </button>
        ))}
        <span className="xc-spacer" />
        {extra}
        <button
          type="button"
          className="xc-mde-mode"
          aria-pressed={preview}
          title={preview ? t("Edit") : t("Preview")}
          onClick={() => setPreview(!preview)}
        >
          {preview ? <Pencil size={14} /> : <Eye size={14} />}
          <span>{preview ? t("Edit") : t("Preview")}</span>
        </button>
      </div>
      {preview ? (
        <Markdown
          className="xc-mde-preview"
          source={value}
          empty={<span className="xc-muted">{t("Nothing to preview")}</span>}
          onToggleTask={(i) => onChange(toggleTask(valueRef.current, i))}
        />
      ) : (
        <textarea
          ref={ref}
          className="xc-mde-input"
          value={value}
          rows={minRows}
          style={{ minHeight: `${minRows * 1.6 + 1.4}em` }}
          placeholder={
            placeholder ?? t("Write something. Markdown is supported.")
          }
          aria-label={label}
          autoFocus={autoFocus}
          onChange={(e) => onChange(e.target.value)}
          onKeyDown={(e) => {
            const mod = e.metaKey || e.ctrlKey;
            if (mod && e.key === "Enter" && onSubmit) {
              e.preventDefault();
              onSubmit();
            } else if (mod && e.key === "b") {
              e.preventDefault();
              tools[1].run();
            } else if (mod && e.key === "i") {
              e.preventDefault();
              tools[2].run();
            }
          }}
        />
      )}
    </div>
  );
}
