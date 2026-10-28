# Constrained rich editor over canonical Markdown

Notes are edited in one WYSIWYG surface backed by canonical Markdown, not in a
source-first editor or a split preview. The rich surface exposes only
constructs that round-trip exactly; anything else is preserved untouched, and
Raw Markdown remains an escape hatch. This keeps editing calm and direct
without letting the editor weaken Markdown portability or silently rewrite
content, including the block markers that carry Memo, Task, and Occurrence
identity.
