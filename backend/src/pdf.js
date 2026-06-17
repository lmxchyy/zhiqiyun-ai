const pdfText = (value) => String(value ?? "")
  .normalize("NFKD")
  .replace(/[^\x20-\x7e]/g, "?")
  .replaceAll("\\", "\\\\")
  .replaceAll("(", "\\(")
  .replaceAll(")", "\\)");

export function buildPdf(presentation) {
  const slides = presentation.slides.length ? presentation.slides : [{ title: presentation.topic, content: "" }];
  const objects = [];
  const add = (value) => {
    objects.push(value);
    return objects.length;
  };

  const fontId = add("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>");
  const contentIds = slides.map((slide, index) => add(
    `<< /Length ${Buffer.byteLength(`BT /F1 24 Tf 54 740 Td (${pdfText(`${index + 1}. ${slide.title}`)}) Tj 0 -48 Td /F1 14 Tf (${pdfText(slide.content)}) Tj ET`)} >>\nstream\nBT /F1 24 Tf 54 740 Td (${pdfText(`${index + 1}. ${slide.title}`)}) Tj 0 -48 Td /F1 14 Tf (${pdfText(slide.content)}) Tj ET\nendstream`
  ));
  const pagesId = objects.length + slides.length + 1;
  const pageIds = slides.map((_, index) => add(
    `<< /Type /Page /Parent ${pagesId} 0 R /MediaBox [0 0 960 540] /Resources << /Font << /F1 ${fontId} 0 R >> >> /Contents ${contentIds[index]} 0 R >>`
  ));
  add(`<< /Type /Pages /Count ${pageIds.length} /Kids [${pageIds.map((id) => `${id} 0 R`).join(" ")}] >>`);
  const catalogId = add(`<< /Type /Catalog /Pages ${pagesId} 0 R >>`);

  const parts = ["%PDF-1.4\n"];
  const offsets = [0];
  objects.forEach((object, index) => {
    offsets[index + 1] = Buffer.byteLength(parts.join(""));
    parts.push(`${index + 1} 0 obj\n${object}\nendobj\n`);
  });
  const xrefOffset = Buffer.byteLength(parts.join(""));
  parts.push(`xref\n0 ${objects.length + 1}\n0000000000 65535 f \n`);
  offsets.slice(1).forEach((offset) => parts.push(`${String(offset).padStart(10, "0")} 00000 n \n`));
  parts.push(`trailer\n<< /Size ${objects.length + 1} /Root ${catalogId} 0 R >>\nstartxref\n${xrefOffset}\n%%EOF`);
  return Buffer.from(parts.join(""));
}
