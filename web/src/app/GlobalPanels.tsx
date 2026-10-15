import VaultPanel from "../features/vault/VaultPanel";
import AssistantPanel from "../features/assistant/AssistantPanel";
import PushSync from "../features/reminders/PushSync";
import FloatingNotes from "../features/notes/FloatingNotes";
import MusicPlayer from "../features/music/PlayerBar";
import SettingsHost from "../features/settings/SettingsHost";
import TimerBar from "../features/recipes/TimerBar";
import PhoneSeen from "../features/me/PhoneSeen";
import HeldNotices from "../features/reminders/HeldNotices";

/*
 * 布局里的全局浮层位置，例如 AI 助手侧边面板（M12）、番茄钟计时条（M11）。
 * 每个面板一行。
 */
export default function GlobalPanels() {
  return (
    <>
      <VaultPanel />
      <AssistantPanel />
      <PushSync />
      <FloatingNotes />
      <MusicPlayer />
      <SettingsHost />
      <TimerBar />
      <PhoneSeen />
      <HeldNotices />
    </>
  );
}
