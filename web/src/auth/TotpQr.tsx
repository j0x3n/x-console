import { useEffect, useState } from "react";
import QRCode from "qrcode";

/** 两步验证的二维码和密钥。初始化页和设置里的“安全”标签共用。 */
export default function TotpQr({
  otpauthUrl,
  secret,
}: {
  otpauthUrl: string;
  secret: string;
}) {
  const [qr, setQr] = useState("");
  useEffect(() => {
    let alive = true;
    QRCode.toDataURL(otpauthUrl, { margin: 1, width: 200 })
      .then((url) => alive && setQr(url))
      .catch(() => alive && setQr(""));
    return () => {
      alive = false;
    };
  }, [otpauthUrl]);
  return (
    <>
      {qr && (
        <img
          src={qr}
          alt="TOTP QR code"
          width={200}
          height={200}
          style={{ display: "block", margin: "0 auto 12px", borderRadius: 6 }}
        />
      )}
      <code className="xc-secret">{secret}</code>
    </>
  );
}
