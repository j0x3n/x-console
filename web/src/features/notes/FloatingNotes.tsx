import { lazy, Suspense } from "react";
import { useFloatingNotes } from "./floating";

// 浮窗里的编辑器按需加载，没开浮窗时不进主包（B72）。
const FloatingWindows = lazy(() => import("./FloatingWindows"));

export default function FloatingNotes() {
  const count = useFloatingNotes((s) => s.ids.length);
  if (count === 0) return null;
  return (
    <Suspense fallback={null}>
      <FloatingWindows />
    </Suspense>
  );
}
