/* 演示用的文件内容：SVG 图片、一个最小的 PDF、文本。都做成 blob 地址。 */

const blobs = new Map<number, string>();

export function setFileUrl(id: number, url: string) {
  blobs.set(id, url);
}

export function fileUrl(id: number) {
  return blobs.get(id);
}

export function svgImage(label: string, from: string, to: string) {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="640" height="420" viewBox="0 0 640 420">
<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="${from}"/><stop offset="1" stop-color="${to}"/></linearGradient></defs>
<rect width="640" height="420" fill="url(#g)"/>
<circle cx="500" cy="110" r="46" fill="#ffffff55"/>
<path d="M0 330 L160 230 L300 320 L420 250 L640 360 L640 420 L0 420 Z" fill="#00000033"/>
<text x="40" y="380" font-family="sans-serif" font-size="30" fill="#fff">${label}</text>
</svg>`;
  return URL.createObjectURL(new Blob([svg], { type: "image/svg+xml" }));
}

export function textFile(text: string, type = "text/plain") {
  return URL.createObjectURL(
    new Blob([text], { type: `${type};charset=utf-8` }),
  );
}

/** 一页的 PDF，只有英文字体，所以内容写英文。 */
export function pdfFile(title: string) {
  const content = `BT /F1 28 Tf 60 740 Td (${title}) Tj ET\nBT /F1 14 Tf 60 700 Td (X Console demo file) Tj ET`;
  const objs = [
    "<< /Type /Catalog /Pages 2 0 R >>",
    "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
    "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
    `<< /Length ${content.length} >>\nstream\n${content}\nendstream`,
    "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
  ];
  let out = "%PDF-1.4\n";
  const offsets: number[] = [];
  objs.forEach((o, i) => {
    offsets.push(out.length);
    out += `${i + 1} 0 obj\n${o}\nendobj\n`;
  });
  const xref = out.length;
  out += `xref\n0 ${objs.length + 1}\n0000000000 65535 f \n`;
  out += offsets
    .map((n) => `${String(n).padStart(10, "0")} 00000 n \n`)
    .join("");
  out += `trailer\n<< /Size ${objs.length + 1} /Root 1 0 R >>\nstartxref\n${xref}\n%%EOF`;
  return URL.createObjectURL(new Blob([out], { type: "application/pdf" }));
}
