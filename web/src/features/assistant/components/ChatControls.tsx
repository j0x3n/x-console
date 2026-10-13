import { Brain, Cpu, ShieldCheck, ShieldAlert } from "lucide-react";
import { errorMessage } from "../../../api/client";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import {
  useAiModels,
  useConversationSettings,
  type AiModel,
  type AiPermission,
  type ChatSettings,
} from "../api";
import Picker from "./Picker";

const PERMISSIONS: { value: AiPermission; label: string; hint: string }[] = [
  {
    value: "manual",
    label: "Manual permission",
    hint: "Ask before every change",
  },
  {
    value: "write",
    label: "Write permission",
    hint: "Changes run, deletes and risky actions ask",
  },
  { value: "all", label: "Allow all", hint: "Valid for 2 hours" },
];
const EFFORTS = [
  { value: "off", label: "Off" },
  { value: "low", label: "Low" },
  { value: "medium", label: "Medium" },
  { value: "high", label: "High" },
];

/**
 * 输入框底部的权限、模型、思考程度（B60，B194 换成小下拉）。
 * 有对话时改了马上存，下一条消息生效；还没有对话时改的是新对话的设置。
 */
export default function ChatControls({
  conversationId,
  value,
  defaultModel,
  onDraft,
}: {
  conversationId: number | null;
  value: ChatSettings & { until?: string | null };
  defaultModel: string;
  onDraft: (next: ChatSettings) => void;
}) {
  const t = useT();
  const models = useAiModels();
  const save = useConversationSettings();

  const change = (patch: Partial<ChatSettings>) => {
    const next = { ...value, ...patch };
    if (conversationId == null) {
      onDraft(next);
      return;
    }
    save.mutate(
      { id: conversationId, body: patch },
      {
        onError: (err) => toast({ message: errorMessage(err), tone: "error" }),
      },
    );
  };

  const all = value.permission === "all";
  const modelOptions = [
    {
      value: "",
      label: t("Default"),
      hint: defaultModel || undefined,
    },
    ...(models.data ?? []).map((m: AiModel) => ({
      value: `${m.providerId}:${m.id}`,
      label: m.name ?? m.id,
      hint: m.name && m.name !== m.id ? m.id : undefined,
    })),
  ];
  const model = modelOptions.find((o) => o.value === value.model);
  const effort = EFFORTS.find((e) => e.value === value.effort);

  return (
    <>
      <Picker
        label={t("Permission")}
        icon={all ? <ShieldAlert size={13} /> : <ShieldCheck size={13} />}
        tone={all ? "warn" : undefined}
        title={
          all ? `${t("Permission")} · ${t("Valid for 2 hours")}` : undefined
        }
        value={value.permission}
        options={PERMISSIONS.map((p) => ({
          value: p.value,
          label: t(p.label),
          hint: t(p.hint),
        }))}
        onChange={(v) => change({ permission: v as AiPermission })}
      />
      <Picker
        label={t("Model")}
        fluid
        icon={<Cpu size={13} />}
        value={value.model}
        display={
          value.model
            ? (model?.label ?? value.model.split(":").slice(1).join(":"))
            : defaultModel || t("Default")
        }
        options={modelOptions}
        onChange={(v) => change({ model: v })}
      />
      <Picker
        label={t("Reasoning effort")}
        icon={<Brain size={13} />}
        value={value.effort}
        display={
          effort ? `${t("Reasoning")} ${t(effort.label)}` : t("Reasoning")
        }
        options={[
          { value: "", label: t("Default") },
          ...EFFORTS.map((e) => ({ value: e.value, label: t(e.label) })),
        ]}
        onChange={(v) => change({ effort: v })}
      />
    </>
  );
}
