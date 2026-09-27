import NavChildLinks from "../../components/layout/NavChildLinks";
import type { NavChildrenProps } from "../../lib/navChildren";
import { useDriveItems } from "./api";

/** 侧边栏“云盘”下面：根目录的文件夹。 */
export default function DriveNavChildren({ onNavigate }: NavChildrenProps) {
  const items = useDriveItems({
    folder: null,
    q: "",
    trash: false,
    hidden: false,
  });
  const folders = (items.data ?? []).filter((i) => i.isDir);
  return (
    <NavChildLinks
      links={folders.map((f) => ({
        key: f.id,
        to: `/drive?folder=${f.id}`,
        label: f.name,
      }))}
      allTo="/drive"
      loading={items.isPending}
      error={items.isError}
      empty="根目录没有文件夹"
      onNavigate={onNavigate}
    />
  );
}
