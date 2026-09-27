/*
 * 自动保存：停止输入 delay 毫秒后保存最新内容。
 * - 保存进行中又有改动时，等这次保存结束再存一次最新的，不会丢字。
 * - 保存失败会保留内容，过一会儿重试。
 * - flush() 立刻保存（切换笔记、离开页面前调用）。
 */

export type SaveState = "idle" | "pending" | "saving" | "saved" | "error";

export interface AutoSaverOptions<T> {
  delay: number;
  retryDelay?: number;
  save: (draft: T) => Promise<unknown>;
  onState?: (state: SaveState) => void;
}

export class AutoSaver<T> {
  private latest: T | null = null;
  private inFlight: Promise<void> | null = null;
  private timer: ReturnType<typeof setTimeout> | undefined;
  private disposed = false;
  state: SaveState = "idle";

  constructor(private opts: AutoSaverOptions<T>) {}

  /** 有没有还没存下来的改动（包括正在保存的）。 */
  get dirty() {
    return this.latest !== null || this.inFlight !== null;
  }

  /** 还没发出去的最新草稿，离开页面时用 keepalive 请求发送。 */
  get unsaved(): T | null {
    return this.latest;
  }

  change(draft: T) {
    this.latest = draft;
    this.setState("pending");
    this.schedule(this.opts.delay);
  }

  /** 立刻保存所有改动，返回时内容已经存好（或者失败）。 */
  async flush(): Promise<void> {
    clearTimeout(this.timer);
    while (this.inFlight) await this.inFlight;
    if (this.latest !== null) await this.run();
  }

  dispose() {
    this.disposed = true;
    clearTimeout(this.timer);
  }

  resume() {
    this.disposed = false;
  }

  private schedule(delay: number) {
    clearTimeout(this.timer);
    if (!this.disposed) this.timer = setTimeout(() => void this.run(), delay);
  }

  private setState(state: SaveState) {
    this.state = state;
    this.opts.onState?.(state);
  }

  private run(): Promise<void> {
    if (this.inFlight) return this.inFlight; // 结束后会自己接着存
    const draft = this.latest;
    if (draft === null) return Promise.resolve();
    this.latest = null;
    this.setState("saving");
    this.inFlight = this.opts
      .save(draft)
      .then(
        () => {
          this.inFlight = null;
          if (this.latest !== null) return this.run();
          this.setState("saved");
        },
        () => {
          this.inFlight = null;
          // 失败时，如果没有更新的内容，就把这次的放回去重试。
          if (this.latest === null) this.latest = draft;
          this.setState("error");
          this.schedule(this.opts.retryDelay ?? 3000);
        },
      );
    return this.inFlight;
  }
}
