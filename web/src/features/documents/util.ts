import { errorMessage } from "../../api/client";
import { toast } from "../../hooks/useToast";

export function showError(err: unknown) {
  toast({ message: errorMessage(err), tone: "error" });
}
