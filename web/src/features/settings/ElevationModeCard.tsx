import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { coreApi } from "../../api/core";
import { unwrap } from "../../api/client";
import { withElevation } from "../../auth/elevation";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import type { components } from "../../api/gen/core";

type Mode = components["schemas"]["ElevationModeBody"]["mode"];

const MODES: { value: Mode; label: string; hint: string }[] = [
  {
    value: "always",
    label: "Every time",
    hint: "Asks again 5 minutes after the last check.",
  },
  {
    value: "30m",
    label: "Once per 30 minutes",
    hint: "Asks again 30 minutes after the last check.",
  },
  {
    value: "session",
    label: "Once per login",
    hint: "Asks once, then not again until you log out.",
  },
  {
    value: "off",
    label: "Verification off",
    hint: "Never asks for ordinary operations.",
  },
];

const key = ["auth", "elevation-mode"] as const;

/** 设置 → 安全 → 敏感操作二次验证（B48）。 */
export default function ElevationModeCard() {
  const t = useT();
  const qc = useQueryClient();
  const mode = useQuery({
    queryKey: key,
    queryFn: () => unwrap(coreApi.GET("/auth/elevation-mode")),
  });
  const save = useMutation({
    mutationFn: (next: Mode) =>
      withElevation(() =>
        unwrap(coreApi.PUT("/auth/elevation-mode", { body: { mode: next } })),
      ),
    onSuccess: (result) => {
      qc.setQueryData(key, result);
      toast(t("Saved"));
    },
  });
  const current = mode.data?.mode;
  return (
    <section className="xc-card">
      <div className="xc-card-head">
        <h2>{t("Verification for sensitive operations")}</h2>
      </div>
      <p className="xc-muted">
        {t(
          "Terminal, commands, deleting files and similar operations ask for your password or code again.",
        )}
      </p>
      <div
        className="elevation-modes"
        role="radiogroup"
        aria-label={t("Verification for sensitive operations")}
      >
        {MODES.map((m) => (
          <label key={m.value} className="elevation-mode">
            <input
              type="radio"
              name="elevation-mode"
              value={m.value}
              checked={current === m.value}
              disabled={!mode.data || save.isPending}
              onChange={() => save.mutate(m.value)}
            />
            <span>
              <strong>{t(m.label)}</strong>
              <small>{t(m.hint)}</small>
            </span>
          </label>
        ))}
      </div>
      {current === "off" && (
        <p className="elevation-warning" role="note">
          {t(
            "If someone gets your login (an unlocked phone, a leaked cookie), they can open the terminal, delete files and delete backups without being asked for a password.",
          )}
        </p>
      )}
      <small className="xc-muted">
        {t(
          "Changing the password or two-step login, this setting, API tokens, Git connections and restoring a backup always ask.",
        )}
      </small>
    </section>
  );
}
