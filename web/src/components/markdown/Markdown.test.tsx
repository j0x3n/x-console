// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import Markdown from "./Markdown";

const table =
  "| 款式 | 尺寸 | 价格 |\n| --- | --- | --- |\n| 素钉 | 6mm | 11 元 |";

afterEach(cleanup);

function draw(snapshot = false) {
  render(
    <MemoryRouter>
      <Markdown
        source={table}
        snapshot={snapshot ? { showHidden: false } : undefined}
      />
    </MemoryRouter>,
  );
  return document.querySelector<HTMLDivElement>(".xc-md-table")!;
}

/** jsdom 不排版，宽度要自己给。 */
function size(el: HTMLElement, clientWidth: number, scrollWidth: number) {
  Object.defineProperty(el, "clientWidth", { value: clientWidth });
  Object.defineProperty(el, "scrollWidth", { value: scrollWidth });
}

describe("B245 表格右边还有列时加 more", () => {
  it("放得下时不加，滑到最右边时去掉", () => {
    const box = draw();
    expect(box.className).not.toContain("more");
    size(box, 300, 600);
    fireEvent.scroll(box);
    expect(box.className).toContain("more");
    box.scrollLeft = 300;
    fireEvent.scroll(box);
    expect(box.className).not.toContain("more");
  });

  it("截图模式不加", () => {
    const box = draw(true);
    size(box, 300, 600);
    fireEvent.scroll(box);
    expect(box.className).not.toContain("more");
  });
});
