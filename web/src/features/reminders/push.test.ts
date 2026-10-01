import { describe, expect, it } from "vitest";
import {
  deviceName,
  platformOf,
  pushService,
  shouldSync,
  subscriptionBody,
  urlBase64ToUint8Array,
} from "./push";

describe("push helpers", () => {
  it("decodes base64url VAPID keys without padding", () => {
    // "hello?" in base64url is aGVsbG8_ (standard base64 would be aGVsbG8/).
    expect(Array.from(urlBase64ToUint8Array("aGVsbG8_"))).toEqual([
      104, 101, 108, 108, 111, 63,
    ]);
    expect(urlBase64ToUint8Array("AQ").length).toBe(1);
  });

  it("keeps only the fields the server accepts", () => {
    const body = subscriptionBody(
      {
        endpoint: "https://push.example/abc",
        expirationTime: null,
        keys: { p256dh: "p", auth: "a" },
      },
      "UA",
    );
    expect(body).toEqual({
      endpoint: "https://push.example/abc",
      keys: { p256dh: "p", auth: "a" },
      userAgent: "UA",
    });
  });
});

describe("push self-check (B34)", () => {
  it("names the push service from the endpoint", () => {
    expect(pushService("https://fcm.googleapis.com/fcm/send/abc")).toBe(
      "google",
    );
    expect(pushService("https://wns2-sg2p.notify.windows.com/w/?token=x")).toBe(
      "microsoft",
    );
    expect(
      pushService("https://updates.push.services.mozilla.com/wpush/v2/x"),
    ).toBe("mozilla");
    expect(pushService("https://web.push.apple.com/QGx")).toBe("apple");
    expect(pushService("https://push.example.com/x")).toBe("other");
    expect(pushService("not a url")).toBe("other");
  });

  it("reads the browser and system from the user agent", () => {
    const edge =
      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36 Edg/140.0.0.0";
    const safari =
      "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1";
    const chrome =
      "Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Mobile Safari/537.36";
    expect(deviceName(edge)).toBe("Edge · Windows");
    expect(deviceName(safari)).toBe("Safari · iOS");
    expect(deviceName(chrome)).toBe("Chrome · Android");
    expect(platformOf(safari)).toBe("ios");
    expect(deviceName("")).toBe("");
  });
});

describe("shouldSync", () => {
  const now = 10_000_000;
  it("没同步过时要同步", () => {
    expect(shouldSync("https://a/1", null, now)).toBe(true);
  });
  it("同一个订阅 1 小时内不重复", () => {
    expect(shouldSync("https://a/1", `${now - 60_000} https://a/1`, now)).toBe(
      false,
    );
    expect(
      shouldSync("https://a/1", `${now - 3_700_000} https://a/1`, now),
    ).toBe(true);
  });
  it("换了订阅马上同步", () => {
    expect(shouldSync("https://a/2", `${now - 60_000} https://a/1`, now)).toBe(
      true,
    );
  });
});
