import { useState, type FormEvent } from "react";
import { errorMessage } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { useCardDecks, useCreateCard, useUpdateCard, type Card } from "./api";
import "./i18n";
import "./cards.css";

/** 手动新建或修改一张卡片。 */
export default function CardDialog({
  card,
  deck: defaultDeck,
  onClose,
}: {
  /** 传了就是修改 */
  card?: Card;
  /** 新建时默认的牌组 */
  deck?: string;
  onClose: () => void;
}) {
  const t = useT();
  const decks = useCardDecks();
  const create = useCreateCard();
  const update = useUpdateCard();
  const [question, setQuestion] = useState(card?.question ?? "");
  const [answer, setAnswer] = useState(card?.answer ?? "");
  const [deck, setDeck] = useState(card?.deck ?? defaultDeck ?? "");
  const busy = create.isPending || update.isPending;

  const submit = (e: FormEvent) => {
    e.preventDefault();
    const done = {
      onSuccess: () => {
        toast(t("Saved"));
        onClose();
      },
      onError: (err: unknown) =>
        toast({ message: errorMessage(err), tone: "error" }),
    };
    if (card)
      update.mutate({ id: card.id, body: { question, answer, deck } }, done);
    else
      create.mutate({ question, answer, deck: deck.trim() || undefined }, done);
  };

  return (
    <Dialog
      open
      onClose={onClose}
      title={card ? t("Edit card") : t("New card")}
      wide
    >
      <form onSubmit={submit}>
        <label className="xc-field">
          <span>{t("Question")}</span>
          <textarea
            className="xc-textarea"
            rows={3}
            value={question}
            onChange={(e) => setQuestion(e.target.value)}
            autoFocus
          />
        </label>
        <label className="xc-field">
          <span>{t("Card answer")}</span>
          <textarea
            className="xc-textarea"
            rows={3}
            value={answer}
            onChange={(e) => setAnswer(e.target.value)}
          />
        </label>
        <label className="xc-field">
          <span>{t("Deck")}</span>
          <input
            className="xc-input"
            list="cards-deck-names-edit"
            value={deck}
            maxLength={60}
            placeholder={t("Default deck")}
            onChange={(e) => setDeck(e.target.value)}
          />
          <datalist id="cards-deck-names-edit">
            {(decks.data ?? []).map((d) => (
              <option key={d.name} value={d.name} />
            ))}
          </datalist>
        </label>
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button
            type="submit"
            className="xc-btn primary"
            disabled={busy || !question.trim() || !answer.trim()}
          >
            {t("Save")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
