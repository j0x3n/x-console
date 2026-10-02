import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type ChangeEvent,
  type KeyboardEvent,
} from "react";
import { useNavigate } from "react-router";
import { Archive, ImagePlus, Palette, Pin, PinOff, Tag, X } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { errorMessage } from "../../api/client";
import MoreMenu from "../../components/ui/MoreMenu";
import { Loading, ErrorState } from "../../components/ui/States";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import {
  attachmentMarkdown,
  thumbnailSrc,
} from "../../components/markdown/upload";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { formatDate } from "../../lib/time";
import ColorPicker from "./ColorPicker";
import { AutoSaver } from "./autosave";
import { noteBgClass } from "./noteColors";
import { parseTags } from "./logic";
import { joinMemo, splitMemo } from "./memoText";
import {
  MAX_ATTACHMENT_BYTES,
  notesKeys,
  uploadAttachment,
  useCreateNote,
  useDeleteNote,
  useNote,
  useUpdateNote,
  type Note,
} from "./api";

interface Draft {
  title: string;
  body: string;
  tags: string[];
}

/**
 * 便签点开后的弹窗，照 Google Keep 的样子：标题、正文直接编辑，下面一排图标。
 * 图片放在最上面，正文是纯文字。要用格式按钮时在“更多”里打开完整编辑器。
 */
export default function MemoEditor({
  id,
  onClose,
}: {
  id: number;
  onClose: () => void;
}) {
  const note = useNote(id);
  if (note.isPending) return <Loading />;
  if (note.isError)
    return <ErrorState error={note.error} onRetry={() => note.refetch()} />;
  return <Editor note={note.data} onClose={onClose} />;
}

function Editor({ note, onClose }: { note: Note; onClose: () => void }) {
  const t = useT();
  const language = useLanguage();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const update = useUpdateNote();
  const remove = useDeleteNote();
  const create = useCreateNote();
  const [draft, setDraft] = useState<Draft>({
    title: note.title,
    body: note.body,
    tags: note.tags,
  });
  const draftRef = useRef(draft);
  draftRef.current = draft;
  const [panel, setPanel] = useState<"color" | "tags" | null>(null);
  const [tagInput, setTagInput] = useState("");
  const [uploading, setUploading] = useState(0);
  const fileRef = useRef<HTMLInputElement>(null);
  const bodyRef = useRef<HTMLTextAreaElement>(null);
  const titleRef = useRef<HTMLTextAreaElement>(null);
  const { images, text } = splitMemo(draft.body);

  const saver = useRef<AutoSaver<Draft>>(null);
  if (!saver.current)
    saver.current = new AutoSaver<Draft>({
      delay: 600,
      save: (d) =>
        update.mutateAsync({
          id: note.id,
          body: { title: d.title, body: d.body, tags: d.tags },
        }),
    });
  useEffect(() => {
    const s = saver.current!;
    s.resume();
    return () => {
      void s.flush();
      s.dispose();
    };
  }, []);
  const edit = (patch: Partial<Draft>) => {
    const next = { ...draftRef.current, ...patch };
    draftRef.current = next;
    setDraft(next);
    saver.current!.change(next);
  };
  const close = async () => {
    await saver.current!.flush();
    onClose();
  };

  // 标题和正文框跟着内容长高，长了由弹窗中间那块滚动
  useLayoutEffect(() => {
    for (const el of [titleRef.current, bodyRef.current]) {
      if (!el) continue;
      el.style.height = "auto";
      el.style.height = `${el.scrollHeight}px`;
    }
  }, [text, draft.title]);

  const quick = (body: Parameters<typeof update.mutate>[0]["body"]) =>
    update.mutate(
      { id: note.id, body },
      { onError: (e) => toast({ message: errorMessage(e), tone: "error" }) },
    );

  const addTags = () => {
    const tags = parseTags(tagInput);
    if (tags.length)
      edit({
        tags: [...draft.tags, ...tags.filter((x) => !draft.tags.includes(x))],
      });
    setTagInput("");
  };

  const onFiles = async (e: ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(e.target.files ?? []);
    e.target.value = "";
    for (const file of files) {
      if (file.size > MAX_ATTACHMENT_BYTES) {
        toast({
          message: `${file.name}: ${t("File is larger than 50 MB")}`,
          tone: "error",
        });
        continue;
      }
      setUploading((n) => n + 1);
      try {
        const a = await uploadAttachment(note.id, file);
        const cur = splitMemo(draftRef.current.body);
        edit({
          body: joinMemo(
            [...cur.images.map((x) => x.line), attachmentMarkdown(a)],
            cur.text,
          ),
        });
      } catch (err) {
        toast({ message: `${file.name}: ${errorMessage(err)}`, tone: "error" });
      } finally {
        setUploading((n) => n - 1);
      }
    }
  };

  return (
    <div className={`notes-keep ${noteBgClass(note.color)}`}>
      <div className="notes-keep-scroll">
        {images.length > 0 && (
          <div className={`notes-keep-images n${Math.min(images.length, 3)}`}>
            {images.map((img, i) => (
              <figure key={img.line}>
                <img src={thumbnailSrc(img.src)} alt={img.alt} />
                <button
                  type="button"
                  className="notes-keep-image-remove"
                  aria-label={t("Remove image")}
                  title={t("Remove image")}
                  onClick={() =>
                    edit({
                      body: joinMemo(
                        images.filter((_, j) => j !== i).map((x) => x.line),
                        text,
                      ),
                    })
                  }
                >
                  <X size={13} />
                </button>
              </figure>
            ))}
          </div>
        )}
        <div className="notes-keep-head">
          <textarea
            ref={titleRef}
            className="notes-keep-title"
            rows={1}
            value={draft.title}
            placeholder={t("Title")}
            aria-label={t("Title")}
            onChange={(e) =>
              edit({ title: e.target.value.replace(/\n/g, " ") })
            }
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                bodyRef.current?.focus();
              }
            }}
          />
          <button
            type="button"
            className="notes-keep-icon"
            aria-label={note.pinned ? t("Unpin") : t("Pin")}
            title={note.pinned ? t("Unpin") : t("Pin")}
            onClick={() => quick({ pinned: !note.pinned })}
          >
            {note.pinned ? <PinOff size={18} /> : <Pin size={18} />}
          </button>
        </div>
        <textarea
          ref={bodyRef}
          className="notes-keep-body"
          value={text}
          placeholder={t("Take a note…")}
          aria-label={t("Memo")}
          rows={3}
          autoFocus
          onChange={(e) =>
            edit({
              body: joinMemo(
                images.map((x) => x.line),
                e.target.value,
              ),
            })
          }
        />
        {draft.tags.length > 0 && (
          <div className="notes-keep-tags">
            {draft.tags.map((tag) => (
              <span key={tag} className="notes-keep-tag">
                #{tag}
                <button
                  type="button"
                  aria-label={`${t("Remove")} ${tag}`}
                  onClick={() =>
                    edit({ tags: draft.tags.filter((x) => x !== tag) })
                  }
                >
                  <X size={11} />
                </button>
              </span>
            ))}
          </div>
        )}
        <p className="notes-keep-time">
          {uploading > 0
            ? `${t("Uploading…")}`
            : `${t("Edited")} ${formatDate(note.updatedAt, language)}`}
        </p>
      </div>
      {panel === "color" && (
        <div className="notes-keep-panel">
          <ColorPicker
            value={note.color}
            onChange={(color) => quick({ color })}
          />
        </div>
      )}
      {panel === "tags" && (
        <div className="notes-keep-panel">
          <input
            className="xc-input"
            autoFocus
            value={tagInput}
            placeholder={t("Add tags, separated by spaces")}
            aria-label={t("Add tags")}
            onChange={(e) => setTagInput(e.target.value)}
            onKeyDown={(e: KeyboardEvent) => {
              if (e.key === "Enter") {
                e.preventDefault();
                addTags();
              }
            }}
            onBlur={addTags}
          />
        </div>
      )}
      <div className="notes-keep-bar">
        <button
          type="button"
          className={`notes-keep-icon${panel === "color" ? " on" : ""}`}
          aria-label={t("Background")}
          title={t("Background")}
          onClick={() => setPanel(panel === "color" ? null : "color")}
        >
          <Palette size={17} />
        </button>
        <button
          type="button"
          className={`notes-keep-icon${panel === "tags" ? " on" : ""}`}
          aria-label={t("Add tags")}
          title={t("Add tags")}
          onClick={() => setPanel(panel === "tags" ? null : "tags")}
        >
          <Tag size={17} />
        </button>
        <button
          type="button"
          className="notes-keep-icon"
          aria-label={t("Add image")}
          title={t("Add image")}
          onClick={() => fileRef.current?.click()}
        >
          <ImagePlus size={17} />
        </button>
        <button
          type="button"
          className="notes-keep-icon"
          aria-label={t("Archive")}
          title={t("Archive")}
          onClick={async () => {
            await saver.current!.flush();
            update.mutate(
              { id: note.id, body: { archived: true } },
              {
                onSuccess: () => {
                  toast({
                    message: t("Archived"),
                    onUndo: () =>
                      update.mutate({ id: note.id, body: { archived: false } }),
                  });
                  onClose();
                },
              },
            );
          }}
        >
          <Archive size={17} />
        </button>
        <MoreMenu
          label={t("More")}
          title={draft.title || t("Memo")}
          items={[
            {
              key: "tags",
              label: t("Add tags"),
              onSelect: () => setPanel("tags"),
            },
            {
              key: "copy",
              label: t("Make a copy"),
              onSelect: () =>
                create.mutate(
                  {
                    title: draft.title,
                    body: draft.body,
                    tags: draft.tags,
                    kind: "memo",
                    color: note.color,
                  },
                  { onSuccess: () => toast(t("Copied")) },
                ),
            },
            {
              key: "full",
              label: t("Open in the full editor"),
              onSelect: async () => {
                await saver.current!.flush();
                qc.invalidateQueries({ queryKey: notesKeys.note(note.id) });
                navigate(`/notes/${note.id}?view=memos`);
                onClose();
              },
            },
            {
              key: "delete",
              label: t("Delete memo"),
              danger: true,
              onSelect: async () => {
                if (
                  await confirmAction({
                    title: t("Delete this memo?"),
                    confirmLabel: t("Delete"),
                  })
                ) {
                  saver.current!.dispose();
                  remove.mutate(note.id, { onSuccess: onClose });
                }
              },
            },
          ]}
        />
        <span className="xc-spacer" />
        <button type="button" className="xc-btn ghost" onClick={close}>
          {t("Close")}
        </button>
      </div>
      <input
        ref={fileRef}
        type="file"
        accept="image/*"
        multiple
        hidden
        onChange={onFiles}
      />
    </div>
  );
}
