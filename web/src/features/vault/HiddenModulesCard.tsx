import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { errorMessage, unwrap } from "../../api/client";
import { navItems } from "../../app/nav";
import { moduleKeys, moduleOfPath, type ModuleId } from "../../app/modules";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { useVaultStatus, vaultApi } from "./api";
import "./i18n";

/*
 * 设置 → 安全里的“隐藏的模块”（B57）。只在隐藏内容解锁时出现，
 * 锁定时整张卡片不渲染，页面上看不出有这个功能。
 */
export default function HiddenModulesCard() {
  const t = useT();
  const status = useVaultStatus();
  const unlocked = !!status.data?.unlocked;
  const qc = useQueryClient();
  const saved = useQuery({
    queryKey: ["vault", "modules"],
    queryFn: () => unwrap(vaultApi.GET("/vault/modules")),
    enabled: unlocked,
    meta: { silentError: true },
  });
  const [picked, setPicked] = useState<ModuleId[]>([]);
  useEffect(() => {
    if (saved.data) setPicked(saved.data.hidden);
  }, [saved.data]);
  const save = useMutation({
    mutationFn: (hidden: ModuleId[]) =>
      unwrap(vaultApi.PUT("/vault/modules", { body: { hidden } })),
    onSuccess: (out) => {
      qc.setQueryData(["vault", "modules"], out);
      qc.invalidateQueries({ queryKey: moduleKeys.available });
      toast(t("Saved"));
    },
    onError: (err) => toast({ message: errorMessage(err), tone: "error" }),
  });
  if (!unlocked || !saved.data) return null;

  const items = navItems
    .map((n) => ({ ...n, id: moduleOfPath(n.path) }))
    .filter((n): n is typeof n & { id: ModuleId } => n.id !== null);
  const toggle = (id: ModuleId) =>
    setPicked((p) => (p.includes(id) ? p.filter((x) => x !== id) : [...p, id]));
  const changed =
    [...picked].sort().join() !== [...saved.data.hidden].sort().join();

  return (
    <div className="xc-card">
      <div className="xc-card-head">
        <h2>{t("Hidden modules")}</h2>
      </div>
      <p className="vault-note">
        {t(
          "While locked, these leave the sidebar, My day and the command palette. Opening their address shows Not found. They come back when you unlock.",
        )}
      </p>
      <div className="vault-modules">
        {items.map((n) => (
          <label key={n.id} className="xc-check">
            <input
              type="checkbox"
              checked={picked.includes(n.id)}
              onChange={() => toggle(n.id)}
            />
            <n.icon size={14} />
            <span>{t(n.label)}</span>
          </label>
        ))}
      </div>
      <div className="xc-dialog-actions">
        <button
          type="button"
          className="xc-btn primary"
          disabled={!changed || save.isPending}
          onClick={() => save.mutate(picked)}
        >
          {t("Save")}
        </button>
      </div>
    </div>
  );
}
