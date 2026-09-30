import ErrorNotices from "../components/ui/ErrorNotices";
import Toast from "../components/ui/Toast";
import { useToastStore } from "../hooks/useToast";

/** 右下角的提示：报错在上（不自动消失），普通提示在下（B41）。 */
export default function NoticeStack() {
  const toast = useToastStore((s) => s.current);
  const hideToast = useToastStore((s) => s.hide);
  return (
    <div className="notice-stack">
      <ErrorNotices />
      {toast && <Toast key={toast.id} toast={toast} hideToast={hideToast} />}
    </div>
  );
}
