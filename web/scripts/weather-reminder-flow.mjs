import assert from "node:assert/strict";

export async function verifyWeatherReminder(page, base, openSettings) {
  const headers = { "X-Requested-With": "x-console" };
  const get = async (path) => {
    const response = await page.request.get(`${base}/api/v1${path}`);
    assert.equal(response.status(), 200, await response.text());
    return response.json();
  };
  const put = async (path, data) => {
    const response = await page.request.put(`${base}/api/v1${path}`, {
      headers,
      data,
    });
    assert.equal(response.status(), 200, await response.text());
  };
  const originalRain = await get("/weather/alert");
  const originalNotify = await get("/weather/notify");
  const dialog = page.getByRole("dialog", { name: "天气设置" });
  const mode = () => dialog.getByLabel("提醒方式");
  const toggle = () =>
    dialog.getByRole("checkbox", { name: "降水提醒", exact: true });
  const save = async () => {
    await dialog.getByRole("button", { name: "保存", exact: true }).click();
    await dialog.waitFor({ state: "hidden" });
  };
  const expectMode = async (value) => {
    await page.waitForFunction(
      ({ value }) =>
        document.querySelector(".weather-dialog select") &&
        [...document.querySelectorAll(".weather-dialog select")].some(
          (s) => s.value === value,
        ),
      { value },
    );
    assert.equal(await mode().inputValue(), value);
  };
  const calls = [];
  const capture = (request) => {
    if (
      request.method() === "PUT" &&
      /\/weather\/(alert|notify)$/.test(request.url())
    )
      calls.push({
        path: new URL(request.url()).pathname,
        body: request.postDataJSON(),
      });
  };
  page.on("request", capture);
  try {
    await put("/weather/alert", { ...originalRain, enabled: true });
    await put("/weather/notify", { ...originalNotify, rainSoon: true });
    await page.reload();
    await openSettings();
    await expectMode("minutely");
    assert.equal(await toggle().isChecked(), true);
    assert.equal(
      await dialog.getByText("降雨提醒", { exact: true }).count(),
      0,
    );
    assert.equal(
      await dialog.getByRole("checkbox", { name: "快下雨下雪时" }).count(),
      0,
    );
    calls.length = 0;
    await save();
    assert.equal((await get("/weather/alert")).enabled, false);
    assert.equal((await get("/weather/notify")).rainSoon, true);
    assert.deepEqual(
      calls.map((c) => c.path),
      ["/api/v1/weather/alert", "/api/v1/weather/notify"],
    );

    await openSettings();
    await mode().selectOption("probability");
    await dialog.getByLabel("看接下来").selectOption("4");
    await dialog.getByLabel("降雨概率达到").selectOption("80");
    calls.length = 0;
    await save();
    assert.deepEqual(
      calls.map((c) => c.path),
      ["/api/v1/weather/notify", "/api/v1/weather/alert"],
    );
    assert.equal((await get("/weather/notify")).rainSoon, false);
    assert.deepEqual(await get("/weather/alert"), {
      enabled: true,
      leadHours: 4,
      threshold: 80,
    });
    await openSettings();
    await expectMode("probability");
    assert.equal(await dialog.getByLabel("降雨概率达到").inputValue(), "80");

    await mode().selectOption("minutely");
    await dialog.getByLabel("提前多久").selectOption("60");
    await save();
    assert.equal((await get("/weather/alert")).enabled, false);
    assert.equal((await get("/weather/notify")).rainLeadMinutes, 60);
    await openSettings();
    await expectMode("minutely");
    await toggle().uncheck();
    assert.equal(await mode().isDisabled(), true);
    await save();
    assert.equal((await get("/weather/alert")).enabled, false);
    assert.equal((await get("/weather/notify")).rainSoon, false);

    await openSettings();
    await toggle().check();
    await mode().selectOption("probability");
    await dialog.getByRole("button", { name: "取消", exact: true }).click();
    await openSettings();
    assert.equal(await toggle().isChecked(), false);
    await toggle().check();
    await mode().selectOption("probability");
    await page.route("**/api/v1/weather/alert", async (route) => {
      if (route.request().method() !== "PUT") return route.continue();
      await route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({ code: "test_failure", message: "测试保存失败" }),
      });
    });
    await dialog.getByRole("button", { name: "保存", exact: true }).click();
    await dialog.getByText("测试保存失败", { exact: true }).waitFor();
    assert.equal(await mode().inputValue(), "probability");
    assert.equal(await toggle().isChecked(), true);
    assert.equal((await get("/weather/notify")).rainSoon, false);
    await page.unroute("**/api/v1/weather/alert");
    await save();
    assert.equal((await get("/weather/alert")).enabled, true);
    assert.equal((await get("/weather/notify")).rainSoon, false);
    const current = await get("/weather/notify");
    for (const key of [
      "warnings",
      "warningMinLevel",
      "earthquakes",
      "quakeMinMagnitude",
      "quakeRadiusKm",
    ])
      assert.equal(current[key], originalNotify[key]);
  } finally {
    page.off("request", capture);
    await page.unroute("**/api/v1/weather/alert");
    if (await dialog.isVisible())
      await dialog.getByRole("button", { name: "取消", exact: true }).click();
    await put("/weather/alert", { ...originalRain, enabled: false });
    await put("/weather/notify", originalNotify);
    await put("/weather/alert", originalRain);
  }
}
