import { useEffect, useRef, type RefObject } from "react";
import { basicSetup } from "codemirror";
import { Compartment, EditorState, type Extension } from "@codemirror/state";
import { EditorView, keymap } from "@codemirror/view";
import {
  HighlightStyle,
  LanguageDescription,
  syntaxHighlighting,
} from "@codemirror/language";
import { languages } from "@codemirror/language-data";
import { unifiedMergeView } from "@codemirror/merge";
import { tags } from "@lezer/highlight";
import { extension } from "../logic";

/*
 * CodeMirror 6 编辑器。这个文件只从查看器里按需加载，不进主包。
 * 颜色全部用 --xc-* 变量，深浅主题都能看。
 */

const theme = EditorView.theme({
  "&": {
    height: "100%",
    color: "var(--xc-text)",
    backgroundColor: "var(--xc-surface)",
    fontSize: "12.5px",
  },
  ".cm-scroller": {
    fontFamily: "var(--xc-mono)",
    lineHeight: "1.55",
  },
  ".cm-content": { caretColor: "var(--xc-text)" },
  ".cm-cursor": { borderLeftColor: "var(--xc-text)" },
  ".cm-gutters": {
    backgroundColor: "var(--xc-surface-2)",
    color: "var(--xc-faint)",
    borderRight: "1px solid var(--xc-border)",
  },
  ".cm-activeLine": { backgroundColor: "var(--xc-surface-2)" },
  ".cm-activeLineGutter": {
    backgroundColor: "var(--xc-surface-2)",
    color: "var(--xc-text-2)",
  },
  "&.cm-focused": { outline: "none" },
  "&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection":
    { backgroundColor: "var(--xc-accent-soft)" },
  ".cm-searchMatch": { backgroundColor: "var(--xc-warn-soft)" },
  ".cm-searchMatch-selected": { backgroundColor: "var(--xc-accent-soft)" },
  ".cm-panels": {
    backgroundColor: "var(--xc-elevated)",
    color: "var(--xc-text)",
  },
  ".cm-panels.cm-panels-bottom": { borderTop: "1px solid var(--xc-border)" },
  ".cm-textfield, .cm-button": {
    fontSize: "12px",
    color: "var(--xc-text)",
    backgroundColor: "var(--xc-surface)",
    backgroundImage: "none",
    border: "1px solid var(--xc-border)",
    borderRadius: "6px",
  },
  ".cm-changedLine": { backgroundColor: "var(--xc-ok-soft)" },
  ".cm-deletedChunk": { backgroundColor: "var(--xc-danger-soft)" },
});

const highlight = HighlightStyle.define([
  { tag: [tags.keyword, tags.operatorKeyword], color: "var(--xc-accent)" },
  { tag: [tags.string, tags.special(tags.string)], color: "var(--xc-ok)" },
  { tag: [tags.number, tags.bool, tags.null], color: "var(--xc-warn)" },
  {
    tag: [tags.comment, tags.lineComment, tags.blockComment],
    color: "var(--xc-faint)",
    fontStyle: "italic",
  },
  {
    tag: [tags.propertyName, tags.attributeName, tags.definition(tags.name)],
    color: "var(--xc-info)",
  },
  {
    tag: [tags.typeName, tags.className, tags.tagName],
    color: "var(--xc-danger)",
  },
  { tag: tags.heading, fontWeight: "600", color: "var(--xc-text)" },
  { tag: tags.link, color: "var(--xc-accent)", textDecoration: "underline" },
  { tag: tags.invalid, color: "var(--xc-danger)" },
]);

/** 扩展名找不到时的备选。 */
const FALLBACK: Record<string, string> = {
  conf: "Properties files",
  cfg: "Properties files",
  env: "Properties files",
  log: "",
};

function findLanguage(name: string): LanguageDescription | null {
  const found = LanguageDescription.matchFilename(languages, name);
  if (found) return found;
  const alias = FALLBACK[extension(name)];
  return alias ? LanguageDescription.matchLanguageName(languages, alias) : null;
}

/** 按文件名加载高亮。找不到就不高亮。 */
function useLanguage(
  view: RefObject<EditorView | null>,
  slot: Compartment,
  name: string,
) {
  useEffect(() => {
    const desc = findLanguage(name);
    if (!desc) return;
    let alive = true;
    desc
      .load()
      .then((support) => {
        if (alive && view.current)
          view.current.dispatch({ effects: slot.reconfigure(support) });
      })
      .catch(() => {
        /* 高亮加载失败就按纯文本显示 */
      });
    return () => {
      alive = false;
    };
  }, [name]);
}

const base: Extension[] = [
  basicSetup,
  theme,
  syntaxHighlighting(highlight),
  EditorView.lineWrapping,
];

export default function CodeEditor({
  name,
  text,
  editable,
  onChange,
  onSave,
}: {
  name: string;
  /** 初始内容。变了（比如重新加载）会替换整个文档。 */
  text: string;
  editable: boolean;
  onChange: (text: string) => void;
  onSave: () => void;
}) {
  const host = useRef<HTMLDivElement>(null);
  const view = useRef<EditorView | null>(null);
  const lang = useRef(new Compartment()).current;
  const mode = useRef(new Compartment()).current;
  const handlers = useRef({ onChange, onSave });
  handlers.current = { onChange, onSave };

  useEffect(() => {
    const v = new EditorView({
      parent: host.current!,
      state: EditorState.create({
        doc: text,
        extensions: [
          keymap.of([
            {
              key: "Mod-s",
              preventDefault: true,
              run: () => {
                handlers.current.onSave();
                return true;
              },
            },
          ]),
          ...base,
          lang.of([]),
          mode.of(modeExtensions(editable)),
          EditorView.updateListener.of((u) => {
            if (u.docChanged) handlers.current.onChange(u.state.doc.toString());
          }),
        ],
      }),
    });
    view.current = v;
    return () => {
      v.destroy();
      view.current = null;
    };
  }, []);

  useLanguage(view, lang, name);

  useEffect(() => {
    const v = view.current;
    if (!v || v.state.doc.toString() === text) return;
    v.dispatch({ changes: { from: 0, to: v.state.doc.length, insert: text } });
  }, [text]);

  useEffect(() => {
    const v = view.current;
    if (!v) return;
    v.dispatch({ effects: mode.reconfigure(modeExtensions(editable)) });
    if (editable) v.focus();
  }, [editable]);

  return <div ref={host} className="drive-code" data-testid="drive-code" />;
}

function modeExtensions(editable: boolean): Extension {
  return [EditorView.editable.of(editable), EditorState.readOnly.of(!editable)];
}

/** 和服务器上的版本对比：删掉的行标红，新加的行标绿。只读。 */
export function DiffEditor({
  name,
  original,
  text,
}: {
  name: string;
  original: string;
  text: string;
}) {
  const host = useRef<HTMLDivElement>(null);
  const view = useRef<EditorView | null>(null);
  const lang = useRef(new Compartment()).current;

  useEffect(() => {
    const v = new EditorView({
      parent: host.current!,
      state: EditorState.create({
        doc: text,
        extensions: [
          ...base,
          lang.of([]),
          modeExtensions(false),
          unifiedMergeView({ original, mergeControls: false }),
        ],
      }),
    });
    view.current = v;
    return () => {
      v.destroy();
      view.current = null;
    };
    // 内容变了由外面换 key 重建。
  }, []);

  useLanguage(view, lang, name);

  return <div ref={host} className="drive-code" />;
}
