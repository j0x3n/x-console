import { create } from "zustand";
import type { HostAgentPermission } from "../assistant/api";

/** B184：服务器 Agent 还没发第一条消息时选的权限，按机器记。只在这次打开面板期间有效。 */
export const useHostDraftMode = create<{
  modes: Record<string, HostAgentPermission>;
  set: (hostId: string, mode: HostAgentPermission) => void;
}>((set) => ({
  modes: {},
  set: (hostId, mode) =>
    set((s) => ({ modes: { ...s.modes, [hostId]: mode } })),
}));
