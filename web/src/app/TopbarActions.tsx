import QuickNote from "../features/notes/QuickNote";
import FocusButton from "../features/calendar/FocusButton";
import TopbarPlayer from "../features/music/TopbarPlayer";
import AssistantButton from "../features/assistant/AssistantButton";
import BackgroundButton from "../features/servers/BackgroundButton";

/*
 * 页头右侧的全局按钮位置。AI 助手（M12）、番茄钟（M11）等全局入口放在这里。
 * 每个入口一行。
 */
export default function TopbarActions() {
  return (
    <>
      <BackgroundButton />
      <TopbarPlayer />
      <QuickNote />
      <FocusButton />
      <AssistantButton />
    </>
  );
}
