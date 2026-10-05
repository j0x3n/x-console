import { useLocation, useNavigate } from "react-router";
import { Cloud, EyeOff, Folder, HardDrive, Link2, Trash2 } from "lucide-react";
import {
  NavPanelGroup,
  NavPanelLink,
  NavPanelSearch,
  NavPanelStack,
} from "../../components/layout/NavPanel";
import { useT } from "../../contexts/LanguageContext";
import type { NavChildrenProps } from "../../lib/navChildren";
import { useVaultUnlocked } from "../vault/api";
import { useDriveItems } from "./api";
import { useRemoteDrives } from "./remote";

/**
 * 左栏“云盘”的二级菜单（B102）：搜索、文件、隐藏（解锁后）、分享、回收站，
 * 下面是根目录的文件夹和备份设置里绑定的网盘。地址参数和云盘页的标签一样。
 */
export default function DriveNavChildren({ onNavigate }: NavChildrenProps) {
  const t = useT();
  const navigate = useNavigate();
  const location = useLocation();
  const params = new URLSearchParams(
    location.pathname === "/drive" ? location.search : "",
  );
  const view = params.get("view");
  const folder = params.get("folder");
  const unlocked = useVaultUnlocked();
  const remotes = useRemoteDrives();
  const items = useDriveItems({
    folder: null,
    q: "",
    trash: false,
    hidden: false,
  });
  const folders = (items.data ?? []).filter((i) => i.isDir);
  const atDrive = location.pathname === "/drive";

  return (
    <NavPanelStack>
      <NavPanelSearch
        placeholder={t("Search files")}
        onSubmit={(q) => {
          navigate(`/drive${q ? `?q=${encodeURIComponent(q)}` : ""}`);
          onNavigate();
        }}
      />
      <NavPanelGroup>
        <NavPanelLink
          to="/drive"
          icon={HardDrive}
          label={t("Files")}
          active={atDrive && !view && !folder}
          onNavigate={onNavigate}
        />
        {unlocked && (
          <NavPanelLink
            to="/drive?view=hidden"
            icon={EyeOff}
            label={t("Hidden items")}
            active={view === "hidden"}
            onNavigate={onNavigate}
          />
        )}
        <NavPanelLink
          to="/drive?view=shares"
          icon={Link2}
          label={t("Shares")}
          active={view === "shares"}
          onNavigate={onNavigate}
        />
        <NavPanelLink
          to="/drive?view=trash"
          icon={Trash2}
          label={t("Trash")}
          active={view === "trash"}
          onNavigate={onNavigate}
        />
      </NavPanelGroup>
      <NavPanelGroup label={t("Folders")}>
        {items.isPending ? (
          <div className="nav-children-note">{t("Loading")}…</div>
        ) : folders.length === 0 ? (
          <div className="nav-children-note">{t("No folders yet")}</div>
        ) : (
          folders.map((f) => (
            <NavPanelLink
              key={f.id}
              to={`/drive?folder=${f.id}`}
              icon={Folder}
              label={f.name}
              active={folder === String(f.id) && !view}
              onNavigate={onNavigate}
            />
          ))
        )}
      </NavPanelGroup>
      {!!remotes.data?.items.length && (
        <NavPanelGroup label={t("Cloud drives")}>
          {remotes.data.items.map((d) => (
            <NavPanelLink
              key={d.id}
              to={`/drive?view=remote&remote=${d.id}`}
              icon={Cloud}
              label={d.name}
              active={
                view === "remote" && params.get("remote") === String(d.id)
              }
              onNavigate={onNavigate}
            />
          ))}
        </NavPanelGroup>
      )}
    </NavPanelStack>
  );
}
