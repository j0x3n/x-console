import { join } from "node:path";

export function watchBrowserErrors(page) {
  const errors = [];
  let rejectFatal;
  const fatal = new Promise((_, reject) => {
    rejectFatal = reject;
  });
  fatal.catch(() => {});
  const report = (message) => {
    errors.push(message);
    rejectFatal(new Error(`页面异常：${message}`));
  };
  const onPageError = (error) => report(error.message);
  const onConsole = (message) => {
    if (
      message.type() === "error" &&
      /TypeError|ReferenceError|SyntaxError|这个页面出错了/.test(message.text())
    )
      report(message.text());
  };
  page.on("pageerror", onPageError);
  page.on("console", onConsole);
  return {
    errors,
    run: (work) => Promise.race([Promise.resolve().then(work), fatal]),
    dispose: () => {
      page.off("pageerror", onPageError);
      page.off("console", onConsole);
    },
  };
}

export async function saveBrowserFailure(page, directory) {
  const text = await page
    .locator("body")
    .innerText({ timeout: 1000 })
    .catch(() => "页面正文无法读取");
  await page
    .screenshot({
      path: join(directory, "failure.png"),
      fullPage: true,
      timeout: 2000,
    })
    .catch(() => {});
  return text.slice(0, 6000);
}
