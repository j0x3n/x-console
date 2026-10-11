/*
 * 浏览器录音。Chrome 出 webm，Safari 出 mp4，所以按浏览器支持的格式挑一个。
 */

const CANDIDATES = [
  "audio/webm;codecs=opus",
  "audio/webm",
  "audio/mp4",
  "audio/ogg;codecs=opus",
];

/** 浏览器能录的第一种格式，一种都没有时返回 undefined（让浏览器自己挑）。 */
export function pickMime(
  isSupported: (type: string) => boolean,
): string | undefined {
  return CANDIDATES.find((type) => isSupported(type));
}

export function canRecord(): boolean {
  return (
    typeof navigator !== "undefined" &&
    !!navigator.mediaDevices?.getUserMedia &&
    typeof MediaRecorder !== "undefined"
  );
}

export interface Recording {
  /** 停止录音，返回录到的内容。 */
  stop: () => Promise<Blob>;
  /** 放弃录音，不返回内容。 */
  cancel: () => void;
}

/** 开始录音。用户拒绝麦克风权限时抛错。 */
export async function startRecording(): Promise<Recording> {
  const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
  const mimeType = pickMime((t) => MediaRecorder.isTypeSupported(t));
  const recorder = new MediaRecorder(
    stream,
    mimeType ? { mimeType } : undefined,
  );
  const chunks: BlobPart[] = [];
  recorder.ondataavailable = (e) => {
    if (e.data.size > 0) chunks.push(e.data);
  };
  const release = () => stream.getTracks().forEach((track) => track.stop());
  recorder.start();
  return {
    stop: () =>
      new Promise<Blob>((resolve) => {
        recorder.onstop = () => {
          release();
          resolve(
            new Blob(chunks, {
              type: recorder.mimeType || mimeType || "audio/webm",
            }),
          );
        };
        recorder.stop();
      }),
    cancel: () => {
      recorder.onstop = null;
      if (recorder.state !== "inactive") recorder.stop();
      release();
    },
  };
}
