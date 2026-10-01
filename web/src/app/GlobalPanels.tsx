import VaultPanel from "../features/vault/VaultPanel";
import AssistantPanel from "../features/assistant/AssistantPanel";
import PushSync from "../features/reminders/PushSync";
import FloatingNotes from "../features/notes/FloatingNotes";

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
    </>
  );
}
