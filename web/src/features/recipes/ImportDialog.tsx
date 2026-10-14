import { useState } from "react";
import { Link, useNavigate } from "react-router";
import { NotebookPen, WandSparkles } from "lucide-react";
import { ApiError, errorMessage } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import { Segmented } from "../../components/ui/Toolbar";
import { useT } from "../../contexts/LanguageContext";
import { useNotes } from "../notes/api";
import { useParseRecipe } from "./api";

type Source = "text" | "note";

/** 从文字导入（B209）：粘贴教程原文，或者选一篇笔记，用 AI 整理成菜谱草稿。 */
export default function ImportDialog({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const t = useT();
  const navigate = useNavigate();
  const parse = useParseRecipe();
  const [source, setSource] = useState<Source>("text");
  const [text, setText] = useState("");
  const [q, setQ] = useState("");

  const close = () => {
    parse.reset();
    onClose();
  };

  const run = () =>
    parse.mutate(
      { text },
      {
        onSuccess: (draft) => {
          close();
          setText("");
          navigate("/recipes/new", { state: { draft } });
        },
      },
    );

  return (
    <Dialog
      open={open}
      onClose={close}
      title={t("Import from text")}
      description={t(
        "AI turns a cooking guide into a recipe. You check it before saving.",
      )}
      wide
      footer={
        <>
          <button className="xc-btn" onClick={close}>
            {t("Cancel")}
          </button>
          {source === "text" && (
            <button
              className="xc-btn primary"
              disabled={!text.trim() || parse.isPending}
              onClick={run}
            >
              <WandSparkles size={14} />
              {parse.isPending ? t("AI is working…") : t("Tidy up with AI")}
            </button>
          )}
        </>
      }
    >
      <Segmented
        label={t("Source")}
        value={source}
        onChange={setSource}
        options={[
          { value: "text", label: t("Paste text") },
          { value: "note", label: t("From a note"), icon: NotebookPen },
        ]}
      />
      {source === "text" ? (
        <div className="xc-field recipes-import-text">
          <textarea
            className="xc-textarea"
            rows={12}
            aria-label={t("Cooking guide")}
            placeholder={t("Paste a cooking guide here")}
            value={text}
            onChange={(e) => setText(e.target.value)}
          />
        </div>
      ) : (
        <NotePicker
          q={q}
          onQ={setQ}
          onPick={(id) => {
            close();
            navigate(`/recipes/new?fromNote=${id}`);
          }}
        />
      )}
      {parse.isError && <ParseError error={parse.error} />}
    </Dialog>
  );
}

/** AI 没配置时给“去设置”，其他错误直接显示原因。 */
export function ParseError({ error }: { error: unknown }) {
  const t = useT();
  const notSet =
    error instanceof ApiError &&
    (error.status === 501 || error.code === "integration_not_configured");
  return (
    <p className="recipes-import-error">
      {notSet ? (
        <>
          {t("AI is not set up yet.")}{" "}
          <Link to="/settings/assistant">{t("Go to settings")}</Link>
        </>
      ) : (
        errorMessage(error)
      )}
    </p>
  );
}

function NotePicker({
  q,
  onQ,
  onPick,
}: {
  q: string;
  onQ: (q: string) => void;
  onPick: (id: number) => void;
}) {
  const t = useT();
  const notes = useNotes({
    q,
    tag: "",
    archived: false,
    pinned: false,
    hidden: false,
    kind: "note",
  });
  const items = (notes.data?.pages ?? []).flatMap((p) => p.items).slice(0, 30);
  return (
    <div className="recipes-import-notes">
      <input
        className="xc-input"
        placeholder={t("Search notes")}
        aria-label={t("Search notes")}
        value={q}
        onChange={(e) => onQ(e.target.value)}
      />
      <ul>
        {items.map((n) => (
          <li key={n.id}>
            <button onClick={() => onPick(n.id)}>
              <NotebookPen size={14} />
              <span>{n.title || t("Untitled")}</span>
            </button>
          </li>
        ))}
        {notes.isSuccess && items.length === 0 && (
          <li className="recipes-import-empty">{t("No note matches")}</li>
        )}
      </ul>
    </div>
  );
}
