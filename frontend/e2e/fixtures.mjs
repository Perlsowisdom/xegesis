// Small book files the tests make themselves, so no real book is needed: an EPUB (a zip with the
// container, package and chapters) and a PDF of plain text pages.

import { deflateRawSync } from "node:zlib";

const CRC = (() => {
  const t = new Uint32Array(256);
  for (let n = 0; n < 256; n++) {
    let c = n;
    for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
    t[n] = c >>> 0;
  }
  return t;
})();

function crc32(buf) {
  let c = 0xffffffff;
  for (const b of buf) c = CRC[(c ^ b) & 0xff] ^ (c >>> 8);
  return (c ^ 0xffffffff) >>> 0;
}

// zip writes the files in order; "mimetype" (an EPUB's first file) is stored, the rest deflated.
export function zip(files) {
  const local = [];
  const central = [];
  let offset = 0;
  for (const [name, text] of files) {
    const data = Buffer.from(text, "utf8");
    const store = name === "mimetype";
    const body = store ? data : deflateRawSync(data);
    const nameBuf = Buffer.from(name, "utf8");
    const head = Buffer.alloc(30);
    head.writeUInt32LE(0x04034b50, 0);
    head.writeUInt16LE(20, 4);
    head.writeUInt16LE(0, 6);
    head.writeUInt16LE(store ? 0 : 8, 8);
    head.writeUInt32LE(0, 10);
    head.writeUInt32LE(crc32(data), 14);
    head.writeUInt32LE(body.length, 18);
    head.writeUInt32LE(data.length, 22);
    head.writeUInt16LE(nameBuf.length, 26);
    head.writeUInt16LE(0, 28);
    local.push(head, nameBuf, body);
    const cen = Buffer.alloc(46);
    cen.writeUInt32LE(0x02014b50, 0);
    cen.writeUInt16LE(20, 4);
    cen.writeUInt16LE(20, 6);
    cen.writeUInt16LE(0, 8);
    cen.writeUInt16LE(store ? 0 : 8, 10);
    cen.writeUInt32LE(0, 12);
    cen.writeUInt32LE(crc32(data), 16);
    cen.writeUInt32LE(body.length, 20);
    cen.writeUInt32LE(data.length, 24);
    cen.writeUInt16LE(nameBuf.length, 28);
    cen.writeUInt32LE(offset, 42);
    central.push(cen, nameBuf);
    offset += head.length + nameBuf.length + body.length;
  }
  const cenSize = central.reduce((n, b) => n + b.length, 0);
  const end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50, 0);
  end.writeUInt16LE(files.length, 8);
  end.writeUInt16LE(files.length, 10);
  end.writeUInt32LE(cenSize, 12);
  end.writeUInt32LE(offset, 16);
  return Buffer.concat([...local, ...central, end]);
}

const para = (i) => `<p>Paragraph ${i} of this chapter speaks of grace and mercy, and of the hope that does not disappoint (Romans 5:5).</p>`;

function chapter(n, title, count) {
  return `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><head><title>${title}</title></head><body>
<h2 id="c${n}">${title}</h2>
<p id="first${n}">The first words of chapter ${n}: all things work together for good.</p>
${n === 1 ? `<p id="more">Read on in <a id="tochap2" href="ch2.xhtml#c2">Chapter Two</a>.</p>` : ""}
${Array.from({ length: count }, (_, i) => para(i + 1)).join("\n")}
</body></html>`;
}

// epub is a two-chapter book, long enough to scroll. Books with other titles get their own identifier.
export function epub(title = "E2E Test Book", { author = "The Test Harness" } = {}) {
  const uid = title === "E2E Test Book" ? "e2e-test-book" : title.toLowerCase().replace(/[^a-z0-9]+/g, "-");
  return zip([
    ["mimetype", "application/epub+zip"],
    ["META-INF/container.xml", `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`],
    ["OEBPS/content.opf", `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="uid">urn:uuid:${uid}</dc:identifier><dc:title>${title}</dc:title>
    <dc:creator>${author}</dc:creator><dc:rights>Public domain</dc:rights>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="c1" href="ch1.xhtml" media-type="application/xhtml+xml"/>
    <item id="c2" href="ch2.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="c1"/><itemref idref="c2"/></spine>
</package>`],
    ["OEBPS/nav.xhtml", `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"><body>
<nav epub:type="toc"><ol><li><a href="ch1.xhtml">Chapter One</a></li><li><a href="ch2.xhtml">Chapter Two</a></li></ol></nav></body></html>`],
    ["OEBPS/ch1.xhtml", chapter(1, "Chapter One", 40)],
    ["OEBPS/ch2.xhtml", chapter(2, "Chapter Two", 10)],
  ]);
}

// pdf is a document of text pages, one line each.
export function pdf(lines = ["Page one: grace and peace.", "Page two: faith, hope and love."]) {
  const parts = ["%PDF-1.4\n"];
  const offsets = [];
  let len = parts[0].length;
  const obj = (body) => {
    offsets.push(len);
    const s = `${offsets.length} 0 obj\n${body}\nendobj\n`;
    parts.push(s);
    len += s.length;
  };
  const kids = lines.map((_, i) => `${4 + 2 * i} 0 R`).join(" ");
  obj("<< /Type /Catalog /Pages 2 0 R >>");
  obj(`<< /Type /Pages /Kids [${kids}] /Count ${lines.length} /MediaBox [0 0 612 792] >>`);
  obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>");
  lines.forEach((l, i) => {
    const content = `BT /F1 18 Tf 72 700 Td (${l}) Tj ET`;
    obj(`<< /Type /Page /Parent 2 0 R /Resources << /Font << /F1 3 0 R >> >> /Contents ${5 + 2 * i} 0 R >>`);
    obj(`<< /Length ${content.length} >>\nstream\n${content}\nendstream`);
  });
  const xref = len;
  parts.push(`xref\n0 ${offsets.length + 1}\n0000000000 65535 f \n`, ...offsets.map((o) => `${String(o).padStart(10, "0")} 00000 n \n`),
    `trailer\n<< /Size ${offsets.length + 1} /Root 1 0 R >>\nstartxref\n${xref}\n%%EOF\n`);
  return Buffer.from(parts.join(""), "latin1");
}

// bibleEpub is a Bible text as a book: Genesis, then the Psalms with each psalm under its own
// heading in one chapter file, and contents entries for books and chapters.
export function bibleEpub(title = "E2E Hebrew Bible") {
  const page = (body) => `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><head><title>t</title></head><body>${body}</body></html>`;
  const filler = (n) => Array.from({ length: n }, (_, i) => `<p>Verse line ${i + 1}: a long line of translated text that fills the page.</p>`).join("\n");
  const psalm = (n, first) => `<h3 id="ps${n}">Psalm ${n}</h3><p>${first}</p>${filler(30)}`;
  return zip([
    ["mimetype", "application/epub+zip"],
    ["META-INF/container.xml", `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`],
    ["OEBPS/content.opf", `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="uid">urn:uuid:e2e-hebrew-bible</dc:identifier><dc:title>${title}</dc:title>
    <dc:creator>The Test Harness</dc:creator><dc:rights>Public domain</dc:rights>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="g" href="gen.xhtml" media-type="application/xhtml+xml"/>
    <item id="p" href="psa.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="g"/><itemref idref="p"/></spine>
</package>`],
    ["OEBPS/nav.xhtml", `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"><body>
<nav epub:type="toc"><ol>
<li><a href="gen.xhtml">The Book of Genesis</a><ol><li><a href="gen.xhtml#g1">Chapter 1</a></li></ol></li>
<li><a href="psa.xhtml">The Book of Psalms</a><ol><li><a href="psa.xhtml#ps83">83</a></li><li><a href="psa.xhtml#ps84">84</a></li><li><a href="psa.xhtml#ps85">85</a></li></ol></li>
</ol></nav></body></html>`],
    ["OEBPS/gen.xhtml", page(`<h2>The Book of Genesis</h2><h3 id="g1">Genesis 1</h3><p>When God began to create heaven and earth.</p>${filler(10)}`)],
    ["OEBPS/psa.xhtml", page(`<h2>The Book of Psalms</h2>${psalm(83, "O God, do not keep silence.")}${psalm(84, "How lovely are Your dwellings, LORD of armies.")}${psalm(85, "You favored Your land, LORD.")}`)],
  ]);
}
