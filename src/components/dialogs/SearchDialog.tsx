import type * as Model from "../../types/domain";
import React, { useState, useMemo } from "react";
import { useT } from "../../contexts/LanguageContext";
import { companyRecords, peopleRecords } from "../../data/workspace";
import { Search, Users, Command, ArrowRight } from "lucide-react";

interface SearchDialogProps {
  close: () => void;
  navigate: Model.Navigate;
  closing: boolean;
}

export default function SearchDialog({
  close,
  navigate,
  closing,
}: SearchDialogProps) {
  const t = useT();
  const [query, setQuery] = useState("");
  const [selectedIndex, setSelectedIndex] = useState(0);
  const options = useMemo(
    () => [
      "Today",
      "Companies",
      "People",
      "Deals",
      "Work",
      "Crew",
      "Settings",
      ...companyRecords.map((x) => x.name),
      ...peopleRecords.map((x) => x.name),
    ],
    [],
  );
  const results = options
    .filter((x) => `${x} ${t(x)}`.toLowerCase().includes(query.toLowerCase()))
    .slice(0, 8);
  return (
    <div
      className={"modal-backdrop" + (closing ? " is-closing" : "")}
      onMouseDown={close}
    >
      <div
        className="command-dialog"
        role="dialog"
        aria-modal="true"
        aria-label={t("Search")}
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="command-input">
          <Search size={18} />
          <input
            autoFocus
            placeholder={t("Search or ask...")}
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              setSelectedIndex(0);
            }}
            onKeyDown={(e) => {
              if (e.key === "ArrowDown") {
                e.preventDefault();
                setSelectedIndex((index) =>
                  Math.min(index + 1, results.length - 1),
                );
              }
              if (e.key === "ArrowUp") {
                e.preventDefault();
                setSelectedIndex((index) => Math.max(index - 1, 0));
              }
              if (e.key === "Enter" && results[selectedIndex]) {
                navigate(results[selectedIndex]);
                close();
              }
            }}
          />
          <kbd>ESC</kbd>
        </div>
        <div className="command-caption">
          {t(query ? "Results" : "Quick navigation")}
        </div>
        <div className="command-results">
          {results.length ? (
            results.map((name, index) => (
              <button
                key={name}
                className={selectedIndex === index ? "is-selected" : ""}
                onMouseEnter={() => setSelectedIndex(index)}
                onClick={() => {
                  navigate(name);
                  close();
                }}
              >
                <span className="result-icon">
                  {[...companyRecords, ...peopleRecords].some(
                    (c) => c.name === name,
                  ) ? (
                    <Users size={15} />
                  ) : (
                    <Command size={15} />
                  )}
                </span>
                {t(name)}
                <ArrowRight size={14} />
              </button>
            ))
          ) : (
            <div className="empty-result">{t("No results found")}</div>
          )}
        </div>
        <div className="command-footer">
          <span>
            <kbd>↑</kbd>
            <kbd>↓</kbd> {t("to navigate")}
          </span>
          <span>
            <kbd>↵</kbd> {t("to open")}
          </span>
        </div>
      </div>
    </div>
  );
}
