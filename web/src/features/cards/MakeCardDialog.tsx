import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { Sparkles } from "lucide-react";
import { errorMessage } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import { Spinner } from "../../components/ui/States";
import { Segmented } from "../../components/ui/Toolbar";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useCardDecks,
  useCardSettings,
  useCreateCard,
  useGenerateCard,
  type CardStyle,
} from "./api";
import { makeBlank } from "./cloze";
import type { CardSource } from "./SelectionCard";
import "./i18n";
import "./cards.css";

const DECK_KEY = "xc.cards.deck";

function lastDeck(): string {
  try {
    return localStorage.getItem(DECK_KEY) ?? "";
  } catch {
    return "";
  }
}

/**
 * 做卡对话框（B130）。选中的文字是上下文。AI 可用时先写一个草稿，
 * 草稿可以改，也可以全部手写；AI 不可用时直接手写。
 */
export default function MakeCardDialog({
  text,
  source,
  onClose,
}: {
  text: string;
  source: CardSource;
  onClose: () => void;
}) {
  const t = useT();
  const settings = useCardSettings();
  const decks = useCardDecks();
  const create = useCreateCard();
  const generate = useGenerateCard();
  const [style, setStyle] = useState<CardStyle>("qa");
  // 手写时先把选中的文字放进答案（问答）或问题（填空），省得再粘贴
  const [question, setQuestion] = useState("");
  const [answer, setAnswer] = useState(text);
  const [deck, setDeck] = useState(lastDeck);
  const questionRef = useRef<HTMLTextAreaElement>(null);
  const started = useRef(false);

  const aiAvailable = settings.data?.aiAvailable ?? false;
  const aiEnabled = settings.data?.aiEnabled ?? true;
  const aiOn = aiAvailable && aiEnabled;

  const draft = (next: CardStyle) => {
    generate.mutate(
      { text, style: next, sourceTitle: source.title || undefined },
      {
        onSuccess: (d) => {
          setQuestion(d.question);
          setAnswer(d.answer);
        },
        onError: (e) => toast({ message: errorMessage(e), tone: "error" }),
      },
    );
  };

  // 设置读到以后，AI 可用就自动写一个草稿（只写一次）
  useEffect(() => {
    if (!aiOn || started.current) return;
    started.current = true;
    draft(style);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [aiOn]);

  // Esc 只关这个对话框，不要连后面的阅读窗口一起关
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      e.stopPropagation();
      onClose();
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [onClose]);

  const changeStyle = (next: CardStyle) => {
    if (next === style) return;
    setStyle(next);
    if (aiOn) {
      draft(next);
      return;
    }
    if (next === "cloze") {
      setQuestion(text);
      setAnswer("");
    } else {
      setQuestion("");
      setAnswer(text);
    }
  };

  const blankOut = () => {
    const box = questionRef.current;
    if (!box) return;
    const r = makeBlank(question, box.selectionStart, box.selectionEnd);
    if (!r) {
      toast({
        message: t("Select the words to hide in the question first"),
        tone: "error",
      });
      return;
    }
    setQuestion(r.question);
    setAnswer(r.answer);
  };

  const save = () => {
    create.mutate(
      {
        question,
        answer,
        deck: deck.trim() || undefined,
        sourceTitle: source.title || undefined,
        sourceLink: source.link || undefined,
      },
      {
        onSuccess: () => {
          try {
            localStorage.setItem(DECK_KEY, deck.trim());
          } catch {
            /* 记不住牌组名也不影响保存 */
          }
          toast(t("Card saved"));
          onClose();
        },
        onError: (e) => toast({ message: errorMessage(e), tone: "error" }),
      },
    );
  };

  const generating = generate.isPending;
  const aiNote = !settings.data
    ? ""
    : !aiAvailable
      ? t("AI is not set up. Write the card yourself.")
      : !aiEnabled
        ? t("AI for cards is off in the card settings.")
        : t(
            "The selected text is sent to your AI provider to write the draft. You can turn this off in the card settings.",
          );

  return createPortal(
    <Dialog
      open
      onClose={onClose}
      title={t("Make a card")}
      wide
      footer={
        <>
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button
            type="button"
            className="xc-btn primary"
            disabled={create.isPending || !question.trim() || !answer.trim()}
            onClick={save}
          >
            {t("Save")}
          </button>
        </>
      }
    >
      <blockquote className="cards-context">{text}</blockquote>
      <div className="cards-make-bar">
        <Segmented<CardStyle>
          label={t("Card style")}
          value={style}
          onChange={changeStyle}
          options={[
            { value: "qa", label: t("Question and answer") },
            { value: "cloze", label: t("Fill in the blank") },
          ]}
        />
        {aiOn && (
          <button
            type="button"
            className="xc-btn small"
            disabled={generating}
            onClick={() => draft(style)}
          >
            {generating ? <Spinner /> : <Sparkles size={14} />}{" "}
            {t("Write with AI")}
          </button>
        )}
      </div>
      {aiNote && <small className="cards-note">{aiNote}</small>}
      <label className="xc-field">
        <span>{t("Question")}</span>
        <textarea
          ref={questionRef}
          className="xc-textarea"
          rows={3}
          value={question}
          onChange={(e) => setQuestion(e.target.value)}
          disabled={generating}
          autoFocus
        />
      </label>
      {style === "cloze" && (
        <button
          type="button"
          className="xc-btn small cards-blank"
          onClick={blankOut}
        >
          {t("Hide the selected words")}
        </button>
      )}
      <label className="xc-field">
        <span>{t("Card answer")}</span>
        <textarea
          className="xc-textarea"
          rows={3}
          value={answer}
          onChange={(e) => setAnswer(e.target.value)}
          disabled={generating}
        />
      </label>
      <label className="xc-field">
        <span>{t("Deck")}</span>
        <input
          className="xc-input"
          list="cards-deck-names"
          value={deck}
          maxLength={60}
          placeholder={t("Default deck")}
          onChange={(e) => setDeck(e.target.value)}
        />
        <datalist id="cards-deck-names">
          {(decks.data ?? []).map((d) => (
            <option key={d.name} value={d.name} />
          ))}
        </datalist>
      </label>
    </Dialog>,
    document.body,
  );
}
