# Strict escape gaps closed (unquoted contexts)

Red proof: run the hardened oracle against the gap payloads below on a build that still used the older unquoted escape rules.

| Context | Attack payload | Pre-fix output | Why exploitable |
|---------|----------------|-----------------|-----------------|
| Unquoted attr after static `/ok/` | `99000\f0` (FORM FEED) | `<div class=/ok/99000\f0>` (seq-len 5→6) | HTML5 treats U+000C as attribute whitespace; only U+0020/`\t`/`\n`/`\r` were entity-escaped. |
| Unquoted `style=width:{{ $x }}` | space / FF in value | seq-len breakout | EscCSS allowed spaces that split unquoted attrs; unquoted style now **forbid**. |
| Unquoted URL at `attr=` (before-value) | `9*000\f` in `poster={{ $a }}{{ $b }}` | first interp used `EscURLAttr` (no unquoted escape) → raw FF splits attrs | `ScanEscapeKind` left `poster=` in `ctxBeforeValue` and returned quoted-URL kind; fixed to `EscURLUnquoted`. |
| Unquoted URL after static `/ok/` lock | `x onmouseover=alert(1)` | `<div href=/ok/x onmouseover=alert(1)>` | Safe URL lock selected `EscHTML`, which does not escape spaces; HTML tokenizer treats the space as starting a new `onmouseover` attribute. |
| Unquoted `@json($x)` | `x onmouseover=alert(1)` | `<div title=&#34;x onmouseover=alert(1)&#34;>` | `@json` was classified as `EscJSONAttr` even in unquoted position; JSON/HTML entities still contain spaces that split attributes. |
| `style="@json($x)"` | `red;background:url(javascript:x)` | style value contains `url(javascript:…)` | `@json` was checked **before** `forbidAttrs["style"]`, so the style forbid never ran. |

Fixes: unquoted URL → `EscURLUnquoted` / `EscUnquoted` after lock; `@json`/`@js` forbidden in unquoted attrs and in `style`; see `TestStrictGaps_Regression`.
