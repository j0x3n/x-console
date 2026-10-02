import {
  useLayoutEffect,
  useRef,
  useState,
  type ClipboardEvent,
  type DragEvent,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";
import {
  Bold,
  Code,
  Eye,
  Heading2,
  ImagePlus,
  Italic,
  Link2,
  List,
  ListChecks,
  Pencil,
  Quote,
  WandSparkles,
} from "lucide-react";
import { errorMessage, isNotLive } from "../../api/client";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import Markdown from "./Markdown";
import InsertMenu from "./InsertMenu";
import PolishDialog from "./PolishDialog";
import type { PolishScene } from "./polish";
import { toggleTask } from "./mdparse";
import {
  insertBlock,
  prefixLines,
  removeBlock,
  wrapSelection,
  type Edit,
} from "./edit";
import {
  IMAGE_TYPES,
  MAX_IMAGE_BYTES,
  attachmentMarkdown,
  uploadFile,
  uploadPlaceholder,
  type UploadScope,
} from "./upload";

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
  /** 传了就能粘贴、拖入、选择图片，上传到公共文件接口的这个分类下 */
  uploadScope?: UploadScope;
  /** 传了就有“AI 润色”按钮，按这个场景用不同的提示词（B56） */
  polish?: PolishScene;
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
  uploadScope,
  polish,
}: MarkdownEditorProps) {
  const t = useT();
  const ref = useRef<HTMLTextAreaElement>(null);
  const fileRef = useRef<HTMLInputElement>(null);
  const [preview, setPreview] = useState(false);
  const [uploading, setUploading] = useState(0);
  const [polishing, setPolishing] = useState(false);
  const valueRef = useRef(value);
  valueRef.current = value;

  // 几张图同时传完时，父组件还没重新渲染，所以这里自己记住最新的内容。
  const change = (text: string) => {
    valueRef.current = text;
    onChange(text);
  };

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
    change(out.text);
    setPreview(false);
    requestAnimationFrame(() => {
      const target = ref.current;
      if (!target) return;
      target.focus();
      target.setSelectionRange(out.start, out.end);
    });
  };

  const upload = async (files: File[]) => {
    if (!uploadScope) return;
    const list = files.filter((f) => {
      if (!IMAGE_TYPES.includes(f.type)) {
        toast({
          message: `${f.name}: ${t("Only images can be added here")}`,
          tone: "error",
        });
        return false;
      }
      if (f.size > MAX_IMAGE_BYTES) {
        toast({
          message: `${f.name}: ${t("Image is larger than 20 MB")}`,
          tone: "error",
        });
        return false;
      }
      return true;
    });
    if (!list.length) return;
    // 先在光标处放占位文字，上传完成后换成真正的地址。
    const tokens = list.map((f) =>
      uploadPlaceholder(
        f.name || "image",
        Math.random().toString(36).slice(2, 7),
      ),
    );
    apply((x, _s, e) => insertBlock(x, e, e, tokens.join("\n\n")));
    setUploading((n) => n + list.length);
    await Promise.all(
      list.map(async (file, i) => {
        try {
          const f = await uploadFile(uploadScope, file);
          change(
            valueRef.current.replace(tokens[i], () => attachmentMarkdown(f)),
          );
        } catch (err) {
          change(removeBlock(valueRef.current, tokens[i]));
          toast({
            message: isNotLive(err)
              ? t("Image upload is not live yet")
              : `${file.name}: ${errorMessage(err)}`,
            tone: "error",
          });
        } finally {
          setUploading((n) => n - 1);
        }
      }),
    );
  };

  const onPaste = (e: ClipboardEvent<HTMLTextAreaElement>) => {
    if (!uploadScope) return;
    const files = Array.from(e.clipboardData.files);
    // 从 Excel、Word 复制时剪贴板里同时有文字和图片，这时按文字粘贴。
    if (!files.length || e.clipboardData.getData("text/plain")) return;
    e.preventDefault();
    void upload(files);
  };

  const onDrop = (e: DragEvent<HTMLTextAreaElement>) => {
    if (!uploadScope) return;
    const files = Array.from(e.dataTransfer.files);
    if (!files.length) return;
    e.preventDefault();
    void upload(files);
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
        <InsertMenu apply={apply} />
        {uploadScope && (
          <button
            type="button"
            title={t("Insert image")}
            aria-label={t("Insert image")}
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => fileRef.current?.click()}
          >
            <ImagePlus size={15} />
          </button>
        )}
        <span className="xc-spacer" />
        {uploading > 0 && (
          <span className="xc-mde-status">{t("Uploading…")}</span>
        )}
        {extra}
        {polish && (
          <button
            type="button"
            title={t("AI polish")}
            aria-label={t("AI polish")}
            onMouseDown={(e) => e.preventDefault()}
            onClick={() =>
              valueRef.current.trim()
                ? setPolishing(true)
                : toast(t("Write something first"))
            }
          >
            <WandSparkles size={15} />
          </button>
        )}
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
          onToggleTask={(i) => change(toggleTask(valueRef.current, i))}
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
          onChange={(e) => change(e.target.value)}
          onPaste={onPaste}
          onDrop={onDrop}
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
      {uploadScope && (
        <input
          ref={fileRef}
          type="file"
          accept={IMAGE_TYPES.join(",")}
          multiple
          hidden
          onChange={(e) => {
            const files = Array.from(e.target.files ?? []);
            e.target.value = "";
            void upload(files);
          }}
        />
      )}
      {polishing &&
        polish &&
        // 挂到 body 上：编辑框常在别的弹窗里，避免被外层的定位和表单影响
        createPortal(
          // 按键不冒泡到外层：外层弹窗按 Esc 会关掉，卡片描述按 Esc 会放弃修改
          <div
            onKeyDown={(e) => {
              e.stopPropagation();
              if (e.key === "Escape") setPolishing(false);
            }}
          >
            <PolishDialog
              text={valueRef.current}
              scene={polish}
              onApply={(text) => {
                change(text);
                setPolishing(false);
              }}
              onClose={() => setPolishing(false)}
            />
          </div>,
          document.body,
        )}
    </div>
  );
}
