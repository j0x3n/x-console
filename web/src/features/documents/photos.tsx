import { useEffect, useMemo, useRef, useState } from "react";
import { ChevronLeft, ChevronRight, ImagePlus, X } from "lucide-react";
import Dialog from "../../components/ui/Dialog";
import { useT } from "../../contexts/LanguageContext";
import { contentUrl } from "../drive/api";
import ImageView from "../drive/viewer/ImageView";
import { useVaultUnlocked } from "../vault/api";
import { useAddDocumentFile, type DocumentFile } from "./api";
import { uploadScan } from "./scans";

/*
 * 档案的照片（用户 2026-10-10 要求）：可以传多张，列表里显示缩略图，点开看大图。
 * 照片和扫描件一样放在云盘的“证件档案”文件夹里，档案里只记云盘文件的编号和名称。
 */

const IMAGE_EXT = /\.(jpe?g|png|gif|webp|bmp|avif)$/i;

export const isPhoto = (f: Pick<DocumentFile, "name">) =>
  IMAGE_EXT.test(f.name);

const thumb = (id: number) => `/api/v1/drive/items/${id}/thumbnail`;

/** 把选好的文件传到云盘，并记到档案上。隐藏内容解锁时存成隐藏文件。 */
export function useAttachFiles() {
  const unlocked = useVaultUnlocked();
  const addFile = useAddDocumentFile();
  return async (docId: number, files: File[]) => {
    for (const file of files) {
      const item = await uploadScan(file, unlocked);
      await addFile.mutateAsync({
        id: docId,
        file: { driveId: item.id, name: item.name },
      });
    }
  };
}

/** 档案详情里的缩略图。点一张打开大图，右上角的叉把它从档案里拿掉。 */
export function PhotoGrid({
  photos,
  onRemove,
}: {
  photos: DocumentFile[];
  onRemove: (f: DocumentFile) => void;
}) {
  const t = useT();
  const [open, setOpen] = useState<number | null>(null);
  if (photos.length === 0) return null;
  const current = open === null ? null : photos[open];
  return (
    <>
      <ul className="documents-photos">
        {photos.map((f, i) => (
          <li key={f.driveId}>
            <button
              type="button"
              className="documents-photo"
              title={f.name}
              aria-label={`${t("View photo")} ${f.name}`}
              onClick={() => setOpen(i)}
            >
              <img src={thumb(f.driveId)} alt={f.name} loading="lazy" />
            </button>
            <button
              type="button"
              className="documents-photo-remove"
              title={t("Remove from this document")}
              aria-label={`${t("Remove from this document")} ${f.name}`}
              onClick={() => onRemove(f)}
            >
              <X size={12} />
            </button>
          </li>
        ))}
      </ul>
      <Dialog
        open={!!current}
        onClose={() => setOpen(null)}
        title={current?.name ?? ""}
        wide
      >
        {current && (
          <div className="documents-viewer">
            <ImageView
              key={current.driveId}
              src={contentUrl(current.driveId, true)}
              alt={current.name}
            />
            {photos.length > 1 && (
              <div className="documents-viewer-nav">
                <button
                  type="button"
                  className="xc-btn small"
                  disabled={open === 0}
                  onClick={() => setOpen((open ?? 0) - 1)}
                >
                  <ChevronLeft size={14} /> {t("Previous photo")}
                </button>
                <span className="xc-muted">
                  {(open ?? 0) + 1} / {photos.length}
                </span>
                <button
                  type="button"
                  className="xc-btn small"
                  disabled={open === photos.length - 1}
                  onClick={() => setOpen((open ?? 0) + 1)}
                >
                  {t("Next photo")} <ChevronRight size={14} />
                </button>
              </div>
            )}
          </div>
        )}
      </Dialog>
    </>
  );
}

/**
 * 新建或修改档案时选照片。选好的先留在这里，保存档案后再一起传。
 * 手机上会让选相册或直接拍照。
 */
export function PhotoPicker({
  files,
  onChange,
}: {
  files: File[];
  onChange: (files: File[]) => void;
}) {
  const t = useT();
  const input = useRef<HTMLInputElement>(null);
  const urls = useMemo(() => files.map((f) => URL.createObjectURL(f)), [files]);
  useEffect(() => () => urls.forEach((u) => URL.revokeObjectURL(u)), [urls]);
  return (
    <div className="xc-field">
      <span>{t("Photos")}</span>
      <div className="documents-picker">
        {files.map((f, i) => (
          <div key={`${f.name}-${i}`} className="documents-picker-item">
            <img src={urls[i]} alt={f.name} />
            <button
              type="button"
              title={t("Remove")}
              aria-label={`${t("Remove")} ${f.name}`}
              onClick={() => onChange(files.filter((_, j) => j !== i))}
            >
              <X size={12} />
            </button>
          </div>
        ))}
        <button
          type="button"
          className="documents-picker-add"
          onClick={() => input.current?.click()}
        >
          <ImagePlus size={18} />
          <span>{t("Add photos")}</span>
        </button>
        <input
          ref={input}
          type="file"
          accept="image/*"
          multiple
          hidden
          data-testid="photo-input"
          onChange={(e) => {
            const picked = Array.from(e.target.files ?? []);
            if (picked.length) onChange([...files, ...picked]);
            e.target.value = "";
          }}
        />
      </div>
      <small>{t("Saved to the drive folder “Documents” when you save.")}</small>
    </div>
  );
}
