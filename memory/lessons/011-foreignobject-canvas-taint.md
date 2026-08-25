# Drawing an SVG-with-foreignObject from a blob: URL taints the canvas

## Lesson

Client-side HTML-to-PNG via `foreignObject` (serialize DOM + CSS into an SVG,
load into an `Image`, draw to canvas, `toBlob`) fails silently in Chromium when
the SVG is loaded from a **blob: URL**: the image loads fine, `drawImage` works,
then `canvas.toBlob` throws `SecurityError: Tainted canvases may not be
exported`. The same SVG over a **data: URL** (base64) exports cleanly.

Symptom trap: `Image.onerror` never fires — the rasterization "succeeds" — so a
`try/catch` around the whole flow returns false with zero console output. Probe
`canvas.toBlob` directly in a minimal case before blaming the SVG markup.

Also: `foreignObject` content must be valid XHTML — serialize the clone with
`XMLSerializer`, never `.outerHTML` (unclosed `<input>`/`<br>` tags make the
image fail to load, again silently).

## Provenance

- Observed 2026-08-25 via in-browser probes on PR "spec 004 S3" (exportpng.js):
  blob-URL variant `tainted: true, "SecurityError: Failed to execute 'toBlob'
  on 'HTMLCanvasElement'"`; data-URL variant `tainted: false, blobSize: 1580`.
  Fix landed in app/js/exportpng.js (commit "feat(timeline): PNG export").
