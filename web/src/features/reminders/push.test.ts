import { describe, expect, it } from "vitest";
import { subscriptionBody, urlBase64ToUint8Array } from "./push";

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
