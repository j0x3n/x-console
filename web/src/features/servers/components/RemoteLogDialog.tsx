import { useState } from "react";
import { isNotLive } from "../../../api/client";
import Dialog from "../../../components/ui/Dialog";
import { NotLive } from "../../../components/ui/States";
import { useT } from "../../../contexts/LanguageContext";
import { RangeLogView } from "../../drive/viewer/LogView";
import { followFilePath, readFileRange, type FileEntry } from "../api";
import "../../drive/drive.css";
import "../../drive/i18n";

/*
 * 在“文件”标签里查看远端的日志和文本（B33）。
 * 和云盘的日志查看一样：先读最后 1 MB，往上滚再读前面的，可以开实时模式。
 */
export default function RemoteLogDialog({
  hostId,
  file,
  onClose,
}: {
  hostId: string;
  file: FileEntry | null;
  onClose: () => void;
}) {
  const t = useT();
  const [notLive, setNotLive] = useState(false);
  const read = (start: number, end?: number) => {
    const stop = end ?? file!.size;
    return readFileRange(
      hostId,
      file!.path,
      start,
      Math.max(1, stop - start),
    ).catch((err: unknown) => {
      if (isNotLive(err)) setNotLive(true);
      throw err;
    });
  };
  return (
    <Dialog
      open={!!file}
      onClose={() => {
        setNotLive(false);
        onClose();
      }}
      title={file?.name ?? ""}
      description={file?.path}
      wide
    >
      {file && (
        <div className="servers-log">
          {notLive ? (
            <NotLive name={t("Viewing remote files")} />
          ) : (
            <RangeLogView
              fileKey={`${hostId}:${file.path}`}
              size={file.size}
              read={read}
              followPath={(offset) => followFilePath(hostId, file.path, offset)}
            />
          )}
        </div>
      )}
    </Dialog>
  );
}
