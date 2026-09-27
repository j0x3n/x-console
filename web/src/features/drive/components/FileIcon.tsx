import {
  File,
  FileArchive,
  FileAudio,
  FileImage,
  FileText,
  FileVideo,
  Folder,
  type LucideIcon,
} from "lucide-react";
import { fileKind, type FileKind } from "../logic";
import type { DriveItem } from "../api";

const ICONS: Record<FileKind, LucideIcon> = {
  folder: Folder,
  image: FileImage,
  pdf: FileText,
  video: FileVideo,
  audio: FileAudio,
  text: FileText,
  archive: FileArchive,
  other: File,
};

export default function FileIcon({
  item,
  size = 16,
}: {
  item: Pick<DriveItem, "isDir" | "mime" | "name">;
  size?: number;
}) {
  const kind = fileKind(item);
  const Icon = ICONS[kind];
  return <Icon size={size} className={`drive-icon ${kind}`} aria-hidden />;
}
