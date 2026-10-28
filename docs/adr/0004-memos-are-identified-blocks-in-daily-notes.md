# Memos are identified blocks inside Daily Notes

Quick capture appends a Memo to the current Daily Note instead of creating a
file per thought, so the user can write into one input without choosing where
it belongs. Each Daily Note has two regions: free-form human writing, which
remains ordinary Markdown with no per-fragment identity, and a Memo stream of
identified blocks that carry their own creation time and may receive Memo
comments. This refines the earlier rule that nothing inside Today has its own
identity: free writing still does not, but Memos do.

## Considered Options

- One file per Memo: simple identity, but floods the Vault with tiny files and
  loses the day as the natural reading unit.
- Headings plus ID comments: cannot reliably delimit arbitrary Markdown.
- Paired block markers (chosen as the candidate): explicit boundaries that
  survive arbitrary content, subject to round-trip validation in the rich
  editor and safe handling inside code blocks.
