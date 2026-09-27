import { Component, useState, type ReactNode } from "react";
import {
  ArrowDown,
  ArrowUp,
  GripVertical,
  Pencil,
  Save,
  X,
} from "lucide-react";
import { ErrorState, Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { errorMessage } from "../../api/client";
import { toast } from "../../hooks/useToast";
import {
  useDashboardLayout,
  useSaveDashboardLayout,
  type DashboardCard,
} from "./api";
import { cardComponents, cardLabels, type CardID } from "./cards";

class CardBoundary extends Component<
  { children: ReactNode },
  { error: Error | null }
> {
  state = { error: null as Error | null };

  static getDerivedStateFromError(error: Error) {
    return { error };
  }

  render() {
    if (this.state.error)
      return (
        <div className="overview-error">
          <span>卡片加载失败：{this.state.error.message}</span>
          <button
            className="xc-btn small"
            onClick={() => this.setState({ error: null })}
          >
            重试
          </button>
        </div>
      );
    return this.props.children;
  }
}

function move(cards: DashboardCard[], from: number, to: number) {
  if (
    from < 0 ||
    to < 0 ||
    from >= cards.length ||
    to >= cards.length ||
    from === to
  )
    return cards;
  const next = [...cards];
  next.splice(to, 0, next.splice(from, 1)[0]);
  return next.map((card, order) => ({ ...card, order }));
}

export default function OverviewPage() {
  const t = useT();
  const layout = useDashboardLayout();
  const save = useSaveDashboardLayout();
  const [draft, setDraft] = useState<DashboardCard[] | null>(null);

  if (layout.isPending)
    return (
      <div className="xc-page">
        <Loading />
      </div>
    );
  if (layout.isError)
    return (
      <div className="xc-page">
        <ErrorState error={layout.error} onRetry={() => layout.refetch()} />
      </div>
    );

  const cards = draft ?? layout.data.cards;
  const persist = () =>
    save.mutate(
      { cards },
      {
        onSuccess: () => {
          setDraft(null);
          toast("布局已保存");
        },
        onError: (error) =>
          toast({ message: errorMessage(error), tone: "error" }),
      },
    );
  const reorder = (from: number, to: number) => setDraft(move(cards, from, to));

  return (
    <div className="xc-page overview-page">
      <h1 className="overview-accessible-title">{t("Overview")}</h1>
      <div className="overview-actions">
        {draft ? (
          <>
            <button className="xc-btn small" onClick={() => setDraft(null)}>
              <X size={14} /> {t("Cancel overview edit")}
            </button>
            <button
              className="xc-btn small primary"
              onClick={persist}
              disabled={save.isPending}
            >
              <Save size={14} /> {t("Save overview")}
            </button>
          </>
        ) : (
          <button
            className="xc-btn small"
            onClick={() =>
              setDraft(layout.data.cards.map((card) => ({ ...card })))
            }
          >
            <Pencil size={14} /> {t("Edit overview")}
          </button>
        )}
      </div>

      {draft && (
        <div className="xc-card overview-editor" aria-label="卡片布局">
          <p className="xc-muted">拖动卡片调整顺序。取消勾选可隐藏卡片。</p>
          <div className="overview-editor-list">
            {cards.map((card, index) => (
              <div
                className="overview-editor-row"
                key={card.id}
                draggable
                onDragStart={(event) =>
                  event.dataTransfer.setData("text/plain", card.id)
                }
                onDragOver={(event) => event.preventDefault()}
                onDrop={(event) => {
                  event.preventDefault();
                  reorder(
                    cards.findIndex(
                      (item) =>
                        item.id === event.dataTransfer.getData("text/plain"),
                    ),
                    index,
                  );
                }}
              >
                <GripVertical size={15} aria-hidden />
                <label>
                  <input
                    type="checkbox"
                    checked={card.visible}
                    onChange={() =>
                      setDraft(
                        cards.map((item) =>
                          item.id === card.id
                            ? { ...item, visible: !item.visible }
                            : item,
                        ),
                      )
                    }
                  />{" "}
                  {t(cardLabels[card.id as CardID])}
                </label>
                <button
                  className="xc-btn small ghost"
                  disabled={index === 0}
                  aria-label={`上移 ${t(cardLabels[card.id as CardID])}`}
                  onClick={() => reorder(index, index - 1)}
                >
                  <ArrowUp size={14} />
                </button>
                <button
                  className="xc-btn small ghost"
                  disabled={index === cards.length - 1}
                  aria-label={`下移 ${t(cardLabels[card.id as CardID])}`}
                  onClick={() => reorder(index, index + 1)}
                >
                  <ArrowDown size={14} />
                </button>
              </div>
            ))}
          </div>
        </div>
      )}

      <div className="overview-grid">
        {cards
          .filter((card) => card.visible)
          .map((card) => {
            const Content = cardComponents[card.id as CardID];
            return (
              <section
                key={card.id}
                className={`overview-card overview-${card.id}`}
                aria-label={t(cardLabels[card.id as CardID])}
              >
                {card.id !== "greeting" && (
                  <div className="overview-card-heading">
                    <h2>{t(cardLabels[card.id as CardID])}</h2>
                  </div>
                )}
                <CardBoundary>
                  <Content />
                </CardBoundary>
              </section>
            );
          })}
      </div>
    </div>
  );
}
