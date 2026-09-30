import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type ClipboardEvent,
  type DragEvent,
  type ReactNode,
} from "react";
import { Link, useNavigate } from "react-router";
import { usePageCrumb } from "../../../stores/page-title";
import { useQueryClient } from "@tanstack/react-query";
import {
  AlarmClock,
  Archive,
  ArchiveRestore,
  ArrowLeft,
  Bold,
  BookOpen,
  Code,
  Columns2,
  Ellipsis,
  Eye,
  EyeOff,
  Heading2,
  ImagePlus,
  Italic,
  Link2,
  List,
  ListChecks,
  ListOrdered,
  Minus,
  Paperclip,
  Pencil,
  Pin,
  PinOff,
  Quote,
  Sparkles,
  SquareKanban,
  Trash2,
  WandSparkles,
  X,
} from "lucide-react";
import { errorMessage } from "../../../api/client";
import Markdown from "../../../components/markdown/Markdown";
import { toggleTask } from "../../../components/markdown/mdparse";
import { ErrorState, Loading } from "../../../components/ui/States";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import { useVaultUnlocked } from "../../vault/api";
import { formatDate, formatTime, relativeTime } from "../../../lib/time";
import {
  MAX_ATTACHMENT_BYTES,
  notesKeys,
  patchNote,
  patchNoteKeepalive,
  uploadAttachment,
  useDeleteNote,
  useDismissSuggestedTags,
  useNote,
  useNoteAiTools,
  useUpdateNote,
  type Note,
} from "../api";
import { AutoSaver, type SaveState } from "../autosave";
import {
  attachmentMarkdown,
  countWords,
  insertBlock,
  parseTags,
  prefixLines,
  removeBlock,
  sameTags,
  uploadPlaceholder,
  wrapSelection,
  type Edit,
} from "../logic";
import ToIssueDialog from "./ToIssueDialog";
import ToReminderDialog from "./ToReminderDialog";
import PolishDialog from "./PolishDialog";
import { confirmAction } from "../../../components/ui/ConfirmDialog";

interface Draft {
  title: string;
  body: string;
  tags: string[];
}

type Mode = "edit" | "split" | "preview";
const MODE_KEY = "xc.notes.mode";

function readMode(): Mode {
  try {
    const v = localStorage.getItem(MODE_KEY);
    return v === "split" || v === "preview" ? v : "edit";
  } catch {
    return "edit";
  }
}

const sameDraft = (a: Draft, b: Draft) =>
  a.title === b.title && a.body === b.body && sameTags(a.tags, b.tags);

export default function NoteEditor({
  id,
  backTo,
}: {
  id: number;
  backTo: string;
}) {
  const note = useNote(id);
  if (note.isPending) return <Loading />;
  if (note.isError)
    return <ErrorState error={note.error} onRetry={() => note.refetch()} />;
  return <EditorBody key={id} note={note.data} backTo={backTo} />;
}

function EditorBody({ note, backTo }: { note: Note; backTo: string }) {
  const t = useT();
  const language = useLanguage();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const update = useUpdateNote();
  const vaultUnlocked = useVaultUnlocked();
  const remove = useDeleteNote();
  const [draft, setDraft] = useState<Draft>({
    title: note.title,
    body: note.body,
    tags: note.tags,
  });
  // 左上角显示“笔记 / 标题”，边打字边更新
  usePageCrumb(draft.title.trim() || t("Untitled note"));
  const draftRef = useRef(draft);
  draftRef.current = draft;
  const [tagInput, setTagInput] = useState("");
  // B32：AI 建议的标签，已经加上的不再显示。
  const dismissSuggested = useDismissSuggestedTags();
  const suggested = (note.suggestedTags ?? []).filter(
    (tag) => !draft.tags.includes(tag),
  );
  const [mode, setModeState] = useState<Mode>(readMode);
  // B40：有内容的笔记打开时先是阅读模式，点“编辑”才进编辑。
  const [reading, setReading] = useState(() => !!note.body.trim());
  const [polishing, setPolishing] = useState(false);
  const ai = useNoteAiTools();
  const [saveState, setSaveState] = useState<SaveState>("idle");
  const [menuOpen, setMenuOpen] = useState(false);
  const [dialog, setDialog] = useState<"issue" | "reminder" | null>(null);
  const [lightbox, setLightbox] = useState<{ src: string; alt: string } | null>(
    null,
  );
  const [uploading, setUploading] = useState(0);
  const [dragging, setDragging] = useState(false);
  const bodyRef = useRef<HTMLTextAreaElement>(null);
  const fileRef = useRef<HTMLInputElement>(null);
  const lastSaved = useRef<Draft>({
    title: note.title,
    body: note.body,
    tags: note.tags,
  });
  const saver = useRef<AutoSaver<Draft>>(null);
  if (!saver.current)
    saver.current = new AutoSaver<Draft>({
      delay: 800,
      onState: setSaveState,
      save: async (d) => {
        const saved = await patchNote(note.id, d);
        lastSaved.current = d;
        qc.setQueryData(notesKeys.note(note.id), saved);
        qc.invalidateQueries({ queryKey: notesKeys.lists });
        qc.invalidateQueries({ queryKey: notesKeys.tags });
      },
    });

  const setMode = (m: Mode) => {
    setModeState(m);
    try {
      localStorage.setItem(MODE_KEY, m);
    } catch {
      /* 存不了就算了 */
    }
  };

  // 另一个窗口改了这条笔记：本地没有未保存的改动时，采用新内容。
  useEffect(() => {
    const server: Draft = {
      title: note.title,
      body: note.body,
      tags: note.tags,
    };
    if (
      saver.current?.dirty ||
      uploading > 0 ||
      sameDraft(server, lastSaved.current)
    )
      return;
    lastSaved.current = server;
    setDraft(server);
  }, [note.title, note.body, note.tags]);

  // 看大图时按 Esc 关闭。
  useEffect(() => {
    if (!lightbox) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setLightbox(null);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [lightbox]);

  // 离开时把没保存的内容存掉。页面关闭用 keepalive 请求。
  useEffect(() => {
    const s = saver.current!;
    s.resume();
    const onHide = () => {
      if (s.unsaved) patchNoteKeepalive(note.id, s.unsaved);
    };
    const onVisibility = () => {
      if (document.visibilityState === "hidden") void s.flush();
    };
    window.addEventListener("pagehide", onHide);
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      window.removeEventListener("pagehide", onHide);
      document.removeEventListener("visibilitychange", onVisibility);
      void s.flush();
      s.dispose();
    };
  }, [note.id]);

  // 编辑模式下文本框随内容长高，整页一起滚动。
  useLayoutEffect(() => {
    const el = bodyRef.current;
    if (!el || mode !== "edit") return;
    el.style.height = "auto";
    el.style.height = `${el.scrollHeight}px`;
  }, [draft.body, mode, reading]);

  const edit = (next: Partial<Draft>) => {
    const merged = { ...draftRef.current, ...next };
    draftRef.current = merged;
    setDraft(merged);
    saver.current!.change(merged);
  };
  const addTags = (input: string) => {
    const tags = parseTags(input);
    if (tags.length)
      edit({
        tags: [...draft.tags, ...tags.filter((x) => !draft.tags.includes(x))],
      });
    setTagInput("");
  };
  // 服务端改了正文（比如转 Issue 后加了链接），直接采用。
  const adopt = (n: Note) => {
    const server = { title: n.title, body: n.body, tags: n.tags };
    lastSaved.current = server;
    setDraft(server);
  };
  const flushThen = async (fn: () => void) => {
    await saver.current!.flush();
    fn();
  };

  /* ---- B40：AI 生成标题、标签，润色正文 ---- */

  const hasBody = !!draft.body.trim();
  const genTitle = () =>
    ai.title.mutate(draftRef.current.body, {
      onSuccess: (out) => out.title && edit({ title: out.title }),
    });
  const genTags = () =>
    ai.tags.mutate(draftRef.current.body, {
      onSuccess: (out) => {
        const current = draftRef.current.tags;
        const added = out.tags.filter((tag) => !current.includes(tag));
        if (added.length) edit({ tags: [...current, ...added] });
      },
    });
  const applyPolish = (body: string) => {
    const before = draftRef.current.body;
    edit({ body });
    setPolishing(false);
    toast({
      message: t("Note polished"),
      onUndo: () => edit({ body: before }),
    });
  };

  /* ---- 工具条 ---- */

  const apply = (fn: (text: string, start: number, end: number) => Edit) => {
    const el = bodyRef.current;
    const text = draftRef.current.body;
    const start = el?.selectionStart ?? text.length;
    const end = el?.selectionEnd ?? text.length;
    const out = fn(text, start, end);
    edit({ body: out.text });
    if (mode === "preview") setMode("edit");
    requestAnimationFrame(() => {
      const target = bodyRef.current;
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

  /* ---- 图片和附件 ---- */

  const upload = async (files: File[]) => {
    const list = files.filter((f) => {
      if (f.size > MAX_ATTACHMENT_BYTES) {
        toast({
          message: `${f.name}: ${t("File is larger than 50 MB")}`,
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
          const a = await uploadAttachment(note.id, file);
          edit({
            body: draftRef.current.body.replace(tokens[i], () =>
              attachmentMarkdown(a),
            ),
          });
        } catch (err) {
          edit({ body: removeBlock(draftRef.current.body, tokens[i]) });
          toast({
            message: `${file.name}: ${errorMessage(err)}`,
            tone: "error",
          });
        } finally {
          setUploading((n) => n - 1);
        }
      }),
    );
  };

  const pickFiles = (images: boolean) => {
    const input = fileRef.current;
    if (!input) return;
    input.accept = images ? "image/*" : "";
    input.click();
  };

  const onPaste = (e: ClipboardEvent) => {
    const files = Array.from(e.clipboardData.files);
    // 从 Excel、Word 复制时剪贴板里同时有文字和图片，这时按文字粘贴。
    if (!files.length || e.clipboardData.getData("text/plain")) return;
    e.preventDefault();
    void upload(files);
  };

  const onDrop = (e: DragEvent) => {
    setDragging(false);
    const files = Array.from(e.dataTransfer.files);
    if (!files.length) return;
    e.preventDefault();
    void upload(files);
  };

  const status =
    uploading > 0
      ? t("Uploading…")
      : saveState === "pending" || saveState === "saving"
        ? t("Saving…")
        : saveState === "error"
          ? t("Save failed. Retrying soon.")
          : saveState === "saved"
            ? t("Saved")
            : `${t("Edited")} ${relativeTime(note.updatedAt, language)}`;

  const preview = (
    <Markdown
      className="notes-preview"
      source={draft.body}
      empty={<span className="xc-muted">{t("Nothing to preview")}</span>}
      onToggleTask={(i) => edit({ body: toggleTask(draftRef.current.body, i) })}
      onImageClick={(src, alt) => setLightbox({ src, alt })}
    />
  );
  const textarea = (
    <textarea
      ref={bodyRef}
      className="notes-body-input"
      value={draft.body}
      placeholder={t("Write something. Markdown is supported.")}
      aria-label={t("Note")}
      autoFocus={!note.body && !!note.title}
      onChange={(e) => edit({ body: e.target.value })}
      onPaste={onPaste}
      onKeyDown={(e) => {
        const mod = e.metaKey || e.ctrlKey;
        if (mod && e.key === "s") {
          e.preventDefault();
          void saver.current!.flush();
        } else if (mod && e.key === "b") {
          e.preventDefault();
          tools[1].run();
        } else if (mod && e.key === "i") {
          e.preventDefault();
          tools[2].run();
        }
      }}
    />
  );

  return (
    <div
      className={`notes-editor ${reading ? "reading" : `mode-${mode}`}${dragging ? " dragging" : ""}`}
      onDragOver={(e) => {
        if (Array.from(e.dataTransfer.types).includes("Files")) {
          e.preventDefault();
          setDragging(true);
        }
      }}
      onDragLeave={(e) => {
        // 移到编辑区里的子元素时不关；移出编辑区或拖出窗口（relatedTarget 为空）时关掉遮罩。
        const to = e.relatedTarget;
        if (!(to instanceof Node) || !e.currentTarget.contains(to))
          setDragging(false);
      }}
      onDrop={onDrop}
    >
      <header className="notes-editor-bar">
        <Link
          to={backTo}
          className="xc-btn ghost small notes-back"
          aria-label={t("Back to list")}
        >
          <ArrowLeft size={15} />
        </Link>
        <span
          className={`notes-save-state ${uploading ? "saving" : saveState}`}
          role="status"
        >
          <i />
          {status}
        </span>
        <span className="xc-spacer" />
        {note.hidden && (
          <span className="xc-badge accent">{t("Hidden note")}</span>
        )}
        {note.archivedAt && (
          <span className="xc-badge warn">{t("Archived")}</span>
        )}
        {reading ? (
          <button
            className="xc-btn small primary notes-edit-btn"
            aria-label={t("Edit")}
            title={t("Edit")}
            onClick={() => {
              setReading(false);
              requestAnimationFrame(() => bodyRef.current?.focus());
            }}
          >
            <Pencil size={14} />
            <span>{t("Edit")}</span>
          </button>
        ) : (
          <div className="notes-modes" role="group" aria-label={t("View")}>
            <button
              className={mode === "edit" ? "on" : ""}
              aria-pressed={mode === "edit"}
              onClick={() => setMode("edit")}
              title={t("Edit")}
            >
              <Pencil size={14} />
              <span>{t("Edit")}</span>
            </button>
            <button
              className={`notes-mode-split ${mode === "split" ? "on" : ""}`}
              aria-pressed={mode === "split"}
              onClick={() => setMode("split")}
              title={t("Side by side")}
            >
              <Columns2 size={14} />
              <span>{t("Side by side")}</span>
            </button>
            <button
              className={mode === "preview" ? "on" : ""}
              aria-pressed={mode === "preview"}
              onClick={() => setMode("preview")}
              title={t("Preview")}
            >
              <Eye size={14} />
              <span>{t("Preview")}</span>
            </button>
          </div>
        )}
        {!reading && (
          <button
            className="xc-btn small notes-done-btn"
            aria-label={t("Done editing")}
            title={t("Done editing")}
            onClick={() => {
              void saver.current!.flush();
              setReading(true);
            }}
          >
            <BookOpen size={14} />
            <span>{t("Done editing")}</span>
          </button>
        )}
        <button
          className={`xc-btn ghost small ${note.pinned ? "notes-pinned" : ""}`}
          onClick={() =>
            update.mutate({ id: note.id, body: { pinned: !note.pinned } })
          }
          aria-pressed={note.pinned}
          title={note.pinned ? t("Unpin") : t("Pin")}
        >
          {note.pinned ? <PinOff size={15} /> : <Pin size={15} />}
        </button>
        <div className="notes-menu-wrap">
          <button
            className="xc-btn ghost small"
            aria-label={t("More")}
            aria-expanded={menuOpen}
            onClick={() => setMenuOpen((v) => !v)}
          >
            <Ellipsis size={15} />
          </button>
          {menuOpen && (
            <>
              <div
                className="notes-menu-backdrop"
                onClick={() => setMenuOpen(false)}
              />
              <div
                className="notes-menu"
                role="menu"
                onClick={() => setMenuOpen(false)}
              >
                <button
                  role="menuitem"
                  disabled={!hasBody}
                  onClick={() => setPolishing(true)}
                >
                  <WandSparkles size={14} /> {t("AI polish")}
                </button>
                <button role="menuitem" onClick={() => pickFiles(false)}>
                  <Paperclip size={14} /> {t("Attach a file")}
                </button>
                <button
                  role="menuitem"
                  onClick={() => flushThen(() => setDialog("issue"))}
                >
                  <SquareKanban size={14} /> {t("Turn into issue")}
                </button>
                <button
                  role="menuitem"
                  onClick={() => flushThen(() => setDialog("reminder"))}
                >
                  <AlarmClock size={14} /> {t("Remind me")}
                </button>
                <button
                  role="menuitem"
                  onClick={() =>
                    update.mutate({
                      id: note.id,
                      body: { archived: !note.archivedAt },
                    })
                  }
                >
                  {note.archivedAt ? (
                    <ArchiveRestore size={14} />
                  ) : (
                    <Archive size={14} />
                  )}
                  {note.archivedAt ? t("Unarchive") : t("Archive")}
                </button>
                {/* 只有解锁隐藏内容后才能改（B13） */}
                {vaultUnlocked && (
                  <button
                    role="menuitem"
                    onClick={() =>
                      update.mutate(
                        { id: note.id, body: { hidden: !note.hidden } },
                        {
                          onSuccess: (n) =>
                            toast(
                              n.hidden
                                ? t("Moved to hidden")
                                : t("No longer hidden"),
                            ),
                        },
                      )
                    }
                  >
                    {note.hidden ? <Eye size={14} /> : <EyeOff size={14} />}
                    {note.hidden ? t("Unhide note") : t("Hide note")}
                  </button>
                )}
                <button
                  role="menuitem"
                  className="danger"
                  onClick={async () => {
                    if (
                      !(await confirmAction({
                        title: t("Delete this note? This cannot be undone."),
                      }))
                    )
                      return;
                    saver.current!.dispose();
                    remove.mutate(note.id, {
                      onSuccess: () => navigate(backTo),
                    });
                  }}
                >
                  <Trash2 size={14} /> {t("Delete")}
                </button>
              </div>
            </>
          )}
        </div>
      </header>

      <div className="notes-editor-scroll">
        <div className="notes-doc">
          {reading ? (
            <h1
              className={`notes-read-title${draft.title.trim() ? "" : " empty"}`}
            >
              {draft.title.trim() || t("Untitled note")}
            </h1>
          ) : (
            <div className="notes-title-row">
              <input
                className="notes-title-input"
                value={draft.title}
                placeholder={t("Title")}
                aria-label={t("Title")}
                autoFocus={!note.title && !note.body}
                onChange={(e) => edit({ title: e.target.value })}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    e.preventDefault();
                    bodyRef.current?.focus();
                  }
                }}
              />
              <button
                type="button"
                className="xc-btn ghost small notes-ai-gen"
                title={t("Generate title with AI")}
                aria-label={t("Generate title with AI")}
                disabled={!hasBody || ai.title.isPending}
                onClick={genTitle}
              >
                <Sparkles
                  size={15}
                  className={ai.title.isPending ? "notes-spin" : ""}
                />
              </button>
            </div>
          )}
          {(!reading || draft.tags.length > 0 || suggested.length > 0) && (
            <div className="notes-meta">
              <div className="notes-tags-edit">
                {draft.tags.map((tag) => (
                  <span key={tag} className="notes-tag">
                    #{tag}
                    {!reading && (
                      <button
                        aria-label={`${t("Remove tag")} ${tag}`}
                        onClick={() =>
                          edit({ tags: draft.tags.filter((x) => x !== tag) })
                        }
                      >
                        <X size={11} />
                      </button>
                    )}
                  </span>
                ))}
                {!reading && (
                  <input
                    value={tagInput}
                    placeholder={draft.tags.length ? t("Add") : t("Add tags")}
                    aria-label={t("Add tags")}
                    onChange={(e) => setTagInput(e.target.value)}
                    onKeyDown={(e) => {
                      if (
                        (e.key === "Enter" || e.key === ",") &&
                        tagInput.trim()
                      ) {
                        e.preventDefault();
                        addTags(tagInput);
                      } else if (
                        e.key === "Backspace" &&
                        !tagInput &&
                        draft.tags.length
                      ) {
                        edit({ tags: draft.tags.slice(0, -1) });
                      }
                    }}
                    onBlur={() => tagInput.trim() && addTags(tagInput)}
                  />
                )}
                {!reading && (
                  <button
                    type="button"
                    className="xc-btn ghost small notes-ai-gen"
                    title={t("Generate tags with AI")}
                    aria-label={t("Generate tags with AI")}
                    disabled={!hasBody || ai.tags.isPending}
                    onClick={genTags}
                  >
                    <Sparkles
                      size={13}
                      className={ai.tags.isPending ? "notes-spin" : ""}
                    />
                  </button>
                )}
                {suggested.length > 0 && (
                  <span
                    className="notes-suggested"
                    title={t("Tags suggested by AI")}
                  >
                    <Sparkles size={12} />
                    {suggested.map((tag) => (
                      <button
                        key={tag}
                        type="button"
                        className="notes-tag suggested"
                        aria-label={`${t("Add tag")} ${tag}`}
                        onClick={() => edit({ tags: [...draft.tags, tag] })}
                      >
                        + #{tag}
                      </button>
                    ))}
                    <button
                      type="button"
                      className="notes-suggested-dismiss"
                      aria-label={t("Dismiss suggested tags")}
                      onClick={() => dismissSuggested.mutate(note.id)}
                    >
                      <X size={11} />
                    </button>
                  </span>
                )}
              </div>
            </div>
          )}

          {/* 预览时只显示渲染后的内容，不显示格式按钮（B23） */}
          {!reading && mode !== "preview" && (
            <div
              className="notes-toolbar"
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
              <span className="notes-toolbar-sep" />
              <button
                type="button"
                title={t("Insert image")}
                aria-label={t("Insert image")}
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => pickFiles(true)}
              >
                <ImagePlus size={15} />
              </button>
              <button
                type="button"
                className="notes-tool-attach"
                title={t("Attach a file")}
                aria-label={t("Attach a file")}
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => pickFiles(false)}
              >
                <Paperclip size={15} />
              </button>
              <span className="notes-toolbar-sep" />
              <button
                type="button"
                className="notes-tool-ai"
                title={t("AI polish")}
                aria-label={t("AI polish")}
                disabled={!hasBody}
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => setPolishing(true)}
              >
                <WandSparkles size={15} />
                <span>{t("AI polish")}</span>
              </button>
            </div>
          )}
          {reading && preview}
          {!reading && mode === "edit" && textarea}
          {!reading && mode === "preview" && preview}
          {!reading && mode === "split" && (
            <div className="notes-split">
              {textarea}
              <div className="notes-split-preview">{preview}</div>
            </div>
          )}
        </div>
      </div>

      <footer className="notes-editor-foot">
        <span>
          {countWords(draft.body)} {t("words")}
        </span>
        <span>
          {t("Created on")} {formatDate(note.createdAt, language)}
        </span>
        <span className="xc-spacer" />
        <span>
          {t("Updated on")} {formatDate(note.updatedAt, language)}{" "}
          {formatTime(note.updatedAt, language)}
        </span>
      </footer>

      {dragging && (
        <div className="notes-drop">
          <ImagePlus size={28} />
          <strong>{t("Drop files to upload")}</strong>
        </div>
      )}

      <input
        ref={fileRef}
        type="file"
        multiple
        hidden
        onChange={(e) => {
          const files = Array.from(e.target.files ?? []);
          e.target.value = "";
          void upload(files);
        }}
      />

      {lightbox && (
        <div
          className="notes-lightbox"
          role="dialog"
          aria-label={lightbox.alt || t("Image")}
          onClick={() => setLightbox(null)}
        >
          <img src={lightbox.src} alt={lightbox.alt} />
          <button
            className="xc-btn small"
            onClick={() => setLightbox(null)}
            aria-label={t("Close")}
          >
            <X size={15} />
          </button>
        </div>
      )}

      {dialog === "issue" && (
        <ToIssueDialog
          open
          onClose={() => setDialog(null)}
          noteId={note.id}
          onDone={adopt}
        />
      )}
      {polishing && (
        <PolishDialog
          body={draft.body}
          onApply={applyPolish}
          onClose={() => setPolishing(false)}
        />
      )}
      {dialog === "reminder" && (
        <ToReminderDialog
          open
          onClose={() => setDialog(null)}
          noteId={note.id}
        />
      )}
    </div>
  );
}
