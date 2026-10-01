import type { CSSProperties } from "react";
import { GitBranch, Github } from "lucide-react";
import { repoColor } from "./logic";

/* 仓库页和设置页共用的小标记（B70）。 */

export function RepoSwatch({ repo }: { repo: string }) {
  return (
    <i
      className="repos-swatch"
      style={{ "--repo": repoColor(repo) } as CSSProperties}
      aria-hidden
    />
  );
}

export function ForgeIcon({
  forge,
  size = 13,
}: {
  forge?: string;
  size?: number;
}) {
  return forge === "forgejo" ? (
    <GitBranch size={size} className="repos-forge" aria-label="Forgejo" />
  ) : (
    <Github size={size} className="repos-forge" aria-label="GitHub" />
  );
}
