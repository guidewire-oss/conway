// PNG export of a rendered DOM block (FR-043, spec 004 Story 3): the pod lens
// and the pod sheet are meeting artefacts, and a screenshot is a poor export.
//
// The block is serialized into an SVG <foreignObject> with the app's own
// stylesheets inlined, drawn onto a canvas at 2x for crispness, and offered as
// a download. No server round-trip, no new dependency — everything needed is
// already in the page.
//
// foreignObject content must be valid XHTML or the image fails to load with no
// diagnostics at all, so the clone is passed through XMLSerializer (which
// closes void elements and quotes attributes), not .outerHTML.

function collectCss() {
  // Only same-origin sheets: cssRules throws on cross-origin access, which is
  // also what would taint the canvas — one try/catch covers both concerns.
  const out = [];
  for (const sheet of document.styleSheets) {
    try {
      out.push([...sheet.cssRules].map((r) => r.cssText).join('\n'));
    } catch { /* cross-origin: skipped */ }
  }
  return out.join('\n');
}

function xhtml(block) {
  const wrap = document.createElementNS('http://www.w3.org/1999/xhtml', 'div');
  wrap.appendChild(block);
  return new XMLSerializer().serializeToString(wrap);
}

export async function exportBlockPNG(block, filename) {
  if (!block) return false;
  try {
    const clone = block.cloneNode(true);
    // Buttons and interactive affordances are not part of the artefact.
    clone.querySelectorAll('button').forEach((b) => b.remove());
    const cs = getComputedStyle(block);
    // The clone renders under <foreignObject>, not the page <body>, so the
    // inherited color and typography it would have gotten from the cascade are
    // absent — without this, dark-theme exports come out with the UA default
    // black-on-white regardless of theme. Copy the block's own computed
    // inheritance onto the wrapper.
    const inherit = [
      'color', 'font-family', 'font-size', 'line-height', 'font-weight',
      'letter-spacing', 'text-transform',
    ].map((k) => cs[k]).join(' ');
    const pad = 12; // mirrored into the SVG's dimensions below, not just the style
    const w = Math.max(1, Math.ceil(block.getBoundingClientRect().width) + pad * 2 || 600);
    const h = Math.max(1, Math.ceil(block.getBoundingClientRect().height) + pad * 2 || 200);
    const bg = cs.getPropertyValue('--panel').trim() || '#ffffff';

    const style = document.createElementNS('http://www.w3.org/1999/xhtml', 'style');
    style.textContent = collectCss().replace(/<\/?style/gi, '');

    const inner = document.createElementNS('http://www.w3.org/1999/xhtml', 'div');
    // padding counts against the declared width so nothing crops; the width is
    // already block-width + 2*pad above.
    inner.setAttribute('style', `width:${w}px;box-sizing:border-box;background:${bg};padding:${pad}px;${inherit}`);
    inner.appendChild(style);
    inner.appendChild(clone);

    const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="${h}">` +
      `<foreignObject width="100%" height="100%">${xhtml(inner)}</foreignObject></svg>`;
    // data: URL, not a blob: URL — Chromium taints a canvas that drew an
    // SVG-with-foreignObject loaded from a blob (observed 2026-08-25), and a
    // tainted canvas refuses toBlob. The same SVG over a data: URL exports fine.
    const dataUrl = 'data:image/svg+xml;base64,' +
      btoa(unescape(encodeURIComponent(svg)));
    const img = await new Promise((resolve, reject) => {
      const i = new Image();
      i.onload = () => resolve(i);
      i.onerror = () => reject(new Error('rasterization failed'));
      i.src = dataUrl;
    });

    const scale = 2; // 2x: deck-paste sharpness without a huge file
    const canvas = document.createElement('canvas');
    canvas.width = w * scale;
    canvas.height = h * scale;
    const ctx = canvas.getContext('2d');
    ctx.fillStyle = bg;
    ctx.fillRect(0, 0, canvas.width, canvas.height);
    ctx.scale(scale, scale);
    ctx.drawImage(img, 0, 0);

    const png = await new Promise((resolve) => canvas.toBlob(resolve, 'image/png'));
    if (!png) return false;
    const a = document.createElement('a');
    a.href = URL.createObjectURL(png);
    a.download = filename;
    a.click();
    setTimeout(() => URL.revokeObjectURL(a.href), 5000);
    return true;
  } catch {
    return false;
  }
}
