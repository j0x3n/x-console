import Dialog from "../../../components/ui/Dialog";
import { useT } from "../../../contexts/LanguageContext";

const SHORTCUTS: [string, string][] = [
  ["C", "New issue"],
  ["J / K", "Select next or previous issue"],
  ["Enter", "Open the selected issue"],
  ["E", "Edit the title"],
  ["1 – 6", "Change status"],
  ["Esc", "Clear selection"],
  ["?", "Show keyboard shortcuts"],
];

/** 看板和列表的快捷键说明，按 ? 打开。 */
export default function ShortcutsDialog({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const t = useT();
  return (
    <Dialog open={open} onClose={onClose} title={t("Keyboard shortcuts")}>
      <dl className="projects-shortcuts">
        {SHORTCUTS.map(([keys, label]) => (
          <div key={keys}>
            <dt>
              <kbd className="projects-kbd">{keys}</kbd>
            </dt>
            <dd>{t(label)}</dd>
          </div>
        ))}
      </dl>
    </Dialog>
  );
}
