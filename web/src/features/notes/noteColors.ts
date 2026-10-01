import type { NoteColor } from "./api";

/*
 * 笔记和便签的背景色（B74）。只存名字，具体颜色在 notes.css 的 .note-bg-<名字> 里，
 * 深浅主题各一套。和 Google Keep 一样 10 种加默认。
 */
export const NOTE_COLORS: { id: NoteColor; label: string }[] = [
  { id: "", label: "Default" },
  { id: "red", label: "Red" },
  { id: "orange", label: "Orange" },
  { id: "yellow", label: "Yellow" },
  { id: "green", label: "Green" },
  { id: "teal", label: "Teal" },
  { id: "blue", label: "Blue" },
  { id: "purple", label: "Purple" },
  { id: "pink", label: "Pink" },
  { id: "brown", label: "Brown" },
  { id: "gray", label: "Gray" },
];

/** 给元素加的类名，没颜色时是空字符串。 */
export function noteBgClass(color?: string): string {
  return color ? `note-bg note-bg-${color}` : "";
}
