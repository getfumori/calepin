# AI reads Human Notes and writes only its own space

Knowledge is split by write authority. Only the user writes Human Notes,
Today, and the Organization Model. AI and external agents may read them under
policy and may write freely in AI Knowledge and Sources, but any change to
human-owned content becomes a Change Proposal that the user reviews. The
first release ships no AI; agents get read access through the API, and the
AI-owned space and Review come later without changing this boundary.

## Considered Options

- Letting agents edit notes directly with Git as the safety net: convenient,
  but makes it impossible to trust that a note says what the user wrote.
- Review for every AI write: safe, but turns AI maintenance into an approval
  queue nobody keeps up with.
