# TextViewer File Icons

- Generated: 2026-11-02.
- Mode: built-in `image_gen`, transparent background; no fallback CLI/API.
- Use case: `logo-brand`.
- The six `*-master.png` files are the original generated images with their alpha channels.
- The application master was a style reference for the text icon; the text file icon was the reference for the other five icons.
- Derived files: `go run ./cmd/assets assets` generates six 256 px PNGs, the light/dark preview, and ten-size Windows ICOs in `internal/fileassoc/icons`. It resizes the artwork without redrawing the designs.
- ICO sizes: 16, 20, 24, 32, 40, 48, 64, 96, 128, 256 px.

| Category | Original output | Master |
| --- | --- | --- |
| Text | `call_0Vw99MksqJrizCrp2KFzBrJP.png` | `text-master.png` |
| Markdown | `call_k6FmActddHVEynCwaJxJKZyg.png` | `markdown-master.png` |
| Code | `call_97vt7kpvoRPyOgEzwuHgXMuC.png` | `code-master.png` |
| Data | `call_k2qMrednKCIg4TMbdrfrsD7Q.png` | `data-master.png` |
| Configuration | `call_VKVJtP4lUqIB8gJZhrqRZAn9.png` | `config-master.png` |
| Script | `call_8Fq1rY26AWcntoC429brIFsK.png` | `script-master.png` |

## Text Prompt

Use case: logo-brand. Asset type: production Windows Explorer file-type icon, first of a coherent six-icon TextViewer family. Input image is a STYLE REFERENCE ONLY: borrow its warm ivory paper, rounded geometry, subtle Fluent depth, rich azure and amber accent; create a DIFFERENT file icon silhouette, not an application tile. Primary request: a single TEXT DOCUMENT file icon. A tall upright warm white sheet with gently rounded bottom corners, a clearly folded top-right corner, a subtle thin azure-blue edge and soft restrained dimensional shading. Three large thick azure horizontal text strokes in the middle, descending lengths, and a small bold amber insertion caret lower-right inside the paper. Composition: square transparent canvas, front-facing flat elevation, sheet occupies about 70% of width and 86% of height, centered with consistent transparent padding, no perspective. Must read crisply at 16, 24, 32 and 48 px; very simple bold shapes, no fine details. No blue square background tile, no separate platform or shadow cast outside silhouette, no tiny app badge, no letters, no words, no extension labels, no watermark, no surrounding objects. Genuine transparent alpha background.

## Markdown Prompt

Use case: logo-brand. Asset type: production Windows Explorer MARKDOWN file icon in the TextViewer six-icon family. Input image is the design reference: preserve exactly the warm ivory upright paper silhouette, thin azure-blue outline, gently rounded corners, large folded top-right corner, front-facing flat elevation, simple Fluent shading, scale, position and transparent padding. Change ONLY the symbols inside the paper: replace all blue text strokes and amber caret with ONE large bold deep TEAL Markdown emblem made of a capital M and a separate downward arrow to its right, centered in the lower middle portion of the sheet. Both glyphs chunky and unmistakable, coherent with rounded geometric stroke weights of the reference, ample spacing and negative space. An extremely small warm amber notch as optional brand accent only if it helps. No extra text, no labels or extension names, no background tile, no outer shadow, no decorative elements, no watermark. Crisp and readable at 16,24,32,48px. Genuine transparent alpha background. Single icon only.

## Code Prompt

Use case: logo-brand. Asset type: production Windows Explorer SOURCE CODE file icon in the TextViewer six-icon family. Input image is the design reference: preserve exactly the warm ivory upright paper silhouette, thin azure-blue outline, gently rounded corners, large folded top-right corner, front-facing flat elevation, simple Fluent shading, scale, position and transparent padding. Change ONLY the symbols inside the paper: replace all text strokes and caret with a large centered CODE emblem composed of two thick deep azure angle brackets facing inward, left < and right >, and a bold short amber forward slash / between them. The exact emblem is </>. Put it in the lower middle portion of the paper clear of the folded corner. Chunky rounded geometric strokes, ample spacing, no tiny features. No words, no extension labels, no extra text, no background tile, no outer shadow, no decorative elements, no watermark. Crisp readable geometry at 16,24,32,48px. Genuine transparent alpha background. Single icon only.

## Data Prompt

Use case: logo-brand. Asset type: production Windows Explorer STRUCTURED DATA file icon in the TextViewer six-icon family. Input image is the design reference: preserve exactly the warm ivory upright paper silhouette, thin azure-blue outline, gently rounded corners, large folded top-right corner, front-facing flat elevation, simple Fluent shading, scale, position and transparent padding. Change ONLY the symbols inside the paper: replace all text strokes and caret with one large bold AMBER-ORANGE pair of curly braces { } and a small dark blue square between them. The braces must be thick rounded geometric glyphs, perfectly clear silhouettes at small sizes. Center the emblem in the lower middle portion of paper clear of folded corner. One strongly simplified emblem only. No words, extension labels, extra text, background tile, outer shadow, decorative elements, watermark. Crisp readable at 16,24,32,48px. Genuine transparent alpha background. Single icon only.

## Configuration Prompt

Use case: logo-brand. Asset type: production Windows Explorer CONFIGURATION file icon in the TextViewer six-icon family. Input image is the design reference: preserve exactly the warm ivory upright paper silhouette, thin azure-blue outline, gently rounded corners, large folded top-right corner, front-facing flat elevation, simple Fluent shading, scale, position and transparent padding. Change ONLY the symbols inside the paper: replace text strokes and caret with a large bold CONFIGURATION emblem made of three horizontal thick dark slate-blue slider rails, each with one chunky rounded knob. Top knob toward left, middle knob toward right, bottom knob near center. Two knobs are azure blue and center knob is warm amber. Keep the emblem very simplified and chunky, three clear rails, no delicate ticks or tiny controls. Center in lower middle paper clear of fold. No text, labels, words, background tile, outer shadow, decorative elements, watermark. Crisp and readable at 16,24,32,48px. Genuine transparent alpha background. Single icon only.

## Script Prompt

Use case: logo-brand. Asset type: production Windows Explorer SCRIPT file icon in the TextViewer six-icon family. Input image is the design reference: preserve exactly the warm ivory upright paper silhouette, thin azure-blue outline, gently rounded corners, large folded top-right corner, front-facing flat elevation, simple Fluent shading, scale, position and transparent padding. Change ONLY the symbols inside the paper: replace all text strokes and caret with one large bold terminal prompt emblem: a forest emerald green greater-than chevron > on the left followed by a thick warm amber underscore _ on the right. The exact symbol is >_ with ample spacing; chunky rounded geometric strokes, not typed thin text. Center the emblem in the lower middle of the sheet, clear of folded corner. No words, extension labels, extra text, background tile, outer shadow, decorative elements, watermark. Crisp readable geometry at 16,24,32,48px. Genuine transparent alpha background. Single icon only.
