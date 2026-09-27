import QuickNote from "../features/notes/QuickNote";
import FocusButton from "../features/calendar/FocusButton";
import AssistantButton from "../features/assistant/AssistantButton";

/*
 * 页头右侧的全局按钮位置。AI 助手（M12）、番茄钟（M11）等全局入口放在这里。
 * 每个入口一行。
 */
export default function TopbarActions() {
  return (
    <>
      <QuickNote />
      <FocusButton />
      <AssistantButton />
    </>
  );
}
