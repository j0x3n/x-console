import type { components } from "../../api/gen/mail";

export type MailAddress = components["schemas"]["MailAddress"];
export type MailAttachmentLike = Pick<
  components["schemas"]["MailAttachment"],
  "index" | "contentId"
>;
