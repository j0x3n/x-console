import { contentUrl, readContent, saveContent } from "../api";
export interface FileSource {
  url: (id: number, inline?: boolean) => string;
  read: typeof readContent;
  save: typeof saveContent;
  followPath?: (id: number, offset: number) => string;
}
export const driveSource: FileSource = {
  url: contentUrl,
  read: readContent,
  save: saveContent,
  followPath: (id, offset) => `/drive/items/${id}/follow?offset=${offset}`,
};
