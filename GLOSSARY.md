# Calepin

A private, self-hosted knowledge system for one person: notes, daily capture,
tasks, and captured Web content, with optional AI that may read but never
silently rewrite what the user wrote.

## Language

### Product and trust

**Self-hosted knowledge system**:
A server-first knowledge system whose data and service lifecycle are controlled
by the user. It may call user-configured AI providers without handing custody
of stored data or credentials to anyone else.
_Avoid_: Cloud notes, local-only notes

**Trusted server**:
The user's own server, trusted to read and process all stored knowledge,
including Sensitive content. Privacy comes from user-controlled hosting and
access, not from hiding content from the server.
_Avoid_: Zero-knowledge server, untrusted cloud

**Plaintext local custody**:
The at-rest boundary in which canonical Markdown, history, indexes, and local
Secret bindings are plaintext on the Trusted server, protected by the
operator's system permissions, disk encryption, and physical security rather
than by application encryption.
_Avoid_: Zero-knowledge storage, application-managed local encryption

**Sensitive content**:
Private material such as credentials, financial records, or intimate journal
entries that may live in a note but needs explicit scoping before it reaches
an AI provider or operational logs.
_Avoid_: End-to-end encrypted content, password vault entry

**AI-redacted span**:
An explicitly marked range of Markdown that the owner reads and searches
normally but that every AI channel sees only as a fixed placeholder. It is
authored through a "Hide from AI" action and is neither encryption nor a
password vault.
_Avoid_: Block ACL, masked-at-rest secret, automatic secret detection

**Content-free operational log**:
Service telemetry limited to identifiers, operation names, status, timings,
sizes, model names, and cost totals. It never contains titles, paths, bodies,
search terms, URLs, prompts, model messages, or secrets.
_Avoid_: AI transcript log, request-body log, remote analytics

**External egress channel**:
Any destination outside the Trusted server that may receive knowledge-derived
data, such as an AI provider, a Web search service, or a backup target. Every
channel is explicit and policy-controlled.
_Avoid_: Hidden network call, model-only disclosure check

**Gateway-protected origin**:
The single-owner Web origin, reachable only behind an operator-controlled
boundary such as an authenticating proxy, VPN, or private network. The product
has no accounts or login of its own; any request that reaches the origin acts
as the owner.
_Avoid_: Public application server, built-in account system

### Clients and channels

**Knowledge client**:
Any client of the Vault Service: the Web client, a future native client, a
browser extension, or the CLI. Every client uses the same HTTP interface and
the same rules.
_Avoid_: Sync client, local Vault copy

**Web client**:
The responsive browser application served by the Vault Service, usable from
desktop and mobile at the same URL.
_Avoid_: Desktop app, offline app

**Ephemeral knowledge client**:
The rule that a browser client may cache only its application shell. Note
bodies, search results, and Vault metadata live only in the open page and are
never persisted in browser storage.
_Avoid_: Offline Vault, browser note cache

**Constrained rich editor**:
The primary note editor. It edits rich blocks directly only for constructs that
round-trip exactly through canonical Markdown; anything else is preserved
untouched or edited through the Raw Markdown escape hatch.
_Avoid_: Split preview, source-first editor, lossy rich text

**Passive attachment**:
A Vault file treated strictly as data. Safe images, audio, video, and PDF may
render inline; everything else downloads, and the server never executes
uploaded content.
_Avoid_: Executable asset, trusted SVG

**Interactive Owner channel**:
The authority of direct human actions such as editing Human Notes, deciding a
Review, or changing Vault configuration. Starting an AI Run from it does not
pass human write authority to the Run.
_Avoid_: AI acting as human, request-supplied actor flag

**Agent channel**:
The authority of external Agents, automation, and every product-owned AI Run.
It may read authorized knowledge and write AI Knowledge, but any change to
Human Notes, Today, or the Organization Model becomes a Change Proposal.
_Avoid_: Human editor, unrestricted machine caller

**Agent API credential**:
A named, revocable machine credential for one external Agent, limited to an
explicit list of Vaults. It always establishes the Agent channel and never
enters a Vault repository.
_Avoid_: Shared global API key, owner credential

**External Agent tool call**:
One bounded, sessionless request from an external Agent runtime that owns its
own conversation and planning. The Vault Service keeps no Agent session for it.
_Avoid_: Hosted Agent session, server-owned Agent plan

**Managed Change Set**:
An all-or-nothing batch from the Agent channel that creates, revises, or
removes AI Knowledge or Sources, with recorded provenance and base revisions.
It cannot touch Human Notes, Today, or the Organization Model.
_Avoid_: Partial batch write, Human Note patch

### Vault, history, and recovery

**Vault**:
The complete isolation unit for one body of knowledge: its canonical Markdown
repository and history, Organization Model, configuration, indexes, AI state,
backup state, and credential references. Nothing scoped to one Vault can reach
another.
_Avoid_: Folder, workspace view, repository alone

**Vault Manifest**:
The canonical document holding a Vault's stable identity, name, format
version, and portable non-secret configuration. Restoring preserves the
identity; forking creates a new one.
_Avoid_: Folder name as identity, secret-bearing config

**Vault context**:
The single explicit Vault identity bound to every route, request, job, and
credential. No operation searches, links, or reads across Vaults.
_Avoid_: Session-global current Vault, cross-Vault query

**Vault Catalog**:
The server-wide registry used to find and open Vaults. It holds identities,
display metadata, and local state, never note content or secrets, and can be
rebuilt by importing repositories.
_Avoid_: Global knowledge database, cross-Vault index

**Primary Vault**:
The Vault holding the user's real, long-lived knowledge.
_Avoid_: Production folder

**Test Vault**:
An ordinary isolated Vault used for experiments and destructive validation
without risk to the Primary Vault.
_Avoid_: Sandbox mode, test folder

**Vault bootstrap**:
Initializing an empty Git repository as a valid Vault by writing its Vault
Manifest and Core Model and creating the first history boundary.
_Avoid_: Vault import, content migration

**Vault import**:
Registering an existing Vault repository, after validation in isolation, as
either a restore or an explicit fork. A plain Markdown folder without a Vault
Manifest is content migration, not Vault import.
_Avoid_: Mounting an arbitrary repository, importing secrets

**Vault restore**:
Rebuilding the same Vault, with the same identity and history, from its
repository. Secret bindings stay unresolved until the owner rebinds them, and
a live Vault with the same identity is never overwritten.
_Avoid_: Import as duplicate, implicit merge

**Vault fork**:
Creating a new Vault identity from an existing Vault or revision, recording
its origin but copying no secrets, runtime state, or backup activation.
_Avoid_: Restore, shared worktree

**Vault trash**:
The reversible first phase of deleting a Vault: it stops all activity and
hides the Vault while keeping its data for a retention period.
_Avoid_: Archive, immediate deletion

**Vault purge**:
The separately confirmed, irreversible deletion of a trashed Vault's local
data. It never deletes a remote backup automatically.
_Avoid_: One-click permanent delete, cascading remote deletion

**Secret binding**:
A server-local mapping from a logical secret reference in the Vault to a real
credential stored outside every Vault repository.
_Avoid_: Secret in Git, copied fork credential

**Repository-complete recovery**:
The guarantee that canonical Markdown, attachments, and Git history alone
reconstruct every knowledge fact, task state, and Organization Model of a
Vault. Indexes, caches, and job state are disposable; secrets are the only
intentional exception.
_Avoid_: Runtime database as source of truth, required index backup

**Repository-native portability**:
The rule that the Git repository itself is the complete portable Vault. There
is no proprietary export format.
_Avoid_: Proprietary Vault bundle, mandatory Export

**Managed repository access**:
The rule that the Vault Service is the only writer to a running Vault. Raw
file edits are accepted only while the service is stopped and are picked up
when it starts.
_Avoid_: Shared writable sync folder, file watcher as concurrency protocol

**Data-only repository**:
The rule that nothing inside a Vault repository is ever executed or trusted as
an instruction, including repository-level Git configuration.
_Avoid_: Executable notebook, repository-controlled process

**Physical placement**:
The mutable repository path of a Durable Object. Moving a file never changes
its identity, type, lifecycle, relationships, or view membership.
_Avoid_: Folder taxonomy, path identity

**Object working revision**:
The content hash of one Durable Object's current bytes. Every write states the
revision it was based on and succeeds only if that is still current.
_Avoid_: Modification time, last-write-wins

**Checkpoint commit**:
A server-created Git commit that seals the current Vault content into durable
history. Saves are immediately durable without one; checkpoints run on a
server schedule and around significant operations, never as a user Save
button.
_Avoid_: Save, manual commit, per-edit commit

**Vault history revision**:
A Git commit marking a durable whole-Vault boundary, used for checkpoints,
citations, cursors, and multi-object operations. Many saves may precede the
next one.
_Avoid_: Autosave token, Object working revision

**Recovery checkpoint**:
The commit created at startup to seal saved changes left uncommitted by a
crash, instead of discarding them.
_Avoid_: Crash rollback, treating HEAD as the only saved state

**Vault transaction**:
An all-or-nothing change to several objects, such as extracting a Memo,
accepting a Change Proposal, or a schema migration. A crash leaves either the
complete before state or the complete after state.
_Avoid_: Sequential write loop, partial migration

**Compensating revision**:
A new save or Vault transaction that undoes earlier content without rewriting
Git history.
_Avoid_: Git reset, force push, history rewrite

**Content conflict**:
The result of saving against a stale Object working revision. The server never
overwrites newer content; the client keeps its draft and offers current,
draft, or manual combination without exposing Git mechanics.
_Avoid_: Git conflict, silent overwrite, CRDT

**Processing cursor**:
The exact Git commit that one incremental purpose, such as Review recent
changes or Knowledge compilation, has fully processed. Each purpose advances
its own cursor only on success.
_Avoid_: Today's files, last modified time

**Remote backup target**:
A configured destination that asynchronously receives committed Vault history
and is verified after each upload. Local work never waits for it, and it is
not a synchronization protocol.
_Avoid_: Client sync, synchronous push

**Remote recovery point**:
The latest checkpoint whose upload to a Remote backup target has been
verified. Losing the server guarantees recovery only up to this point.
_Avoid_: Latest autosave guarantee, last attempted upload

**Backup health**:
The content-free status of a Remote backup target: latest checkpoint, latest
Remote recovery point, lag, and last error.
_Avoid_: Backup file browser, content-bearing error

**Remote backup divergence**:
The safety stop entered when a remote's history no longer extends the expected
Remote recovery point. Backup never merges or force-pushes; the owner decides
how to recover.
_Avoid_: Automatic remote merge, silent force push

**Encrypted Git remote**:
A Remote backup target that stores Vault history so the remote host sees no
plaintext content, paths, or messages, while the local repository stays
ordinary Git.
_Avoid_: Private Git hosting alone, second history model

**Backup Recovery Kit**:
A small owner-held artifact with everything except secrets needed to restore
from an Encrypted Git remote. Backup is not recovery-ready until the owner has
saved a current kit.
_Avoid_: Vault export, passphrase file

**Recovery verification**:
The checks that prove backups are restorable: every upload is verified, and
optional restore tests rebuild a backup in isolation.
_Avoid_: Unverified successful push

**Git history retention**:
The rule that all reachable history is kept. Deleting a note adds a new state
but never erases older content; erasure requires an explicit history rewrite.
_Avoid_: Backup retention window, delete means purge

### Organization Model

**Organization Model**:
The Vault-owned, editable definitions of Types, properties, relationships,
lifecycle, templates, and Saved views, versioned in the same repository as
the knowledge they describe.
_Avoid_: Folder structure, hard-coded ontology

**Core Model**:
The minimal Organization Model in every Vault: the core properties, the
captured/organized/archived Knowledge lifecycle, the Inbox, Daily Notes, and
the task Types. A Vault may customize it.
_Avoid_: Hidden preset, empty schema

**Blank Vault**:
A Vault created with only the Core Model, imposing no organization style.
_Avoid_: Empty repository, schema-free Vault

**Preset**:
An optional starting extension to the Core Model, copied into a Vault at
creation and owned by that Vault afterward.
_Avoid_: Plugin, permanent dependency

**Schema key**:
A human-readable identifier such as `project` or `belongs_to`, written in
Markdown, that is the identity of a Type, property, relationship, or state.
_Avoid_: Hidden definition ID

**Primary type**:
The single optional `type` of a Durable Object, describing what it
fundamentally is. Further classification uses properties, tags, or
relationships.
_Avoid_: Multiple types, inheritance

**Type definition**:
The Organization Model document for one Type: its properties, defaults,
rules, and optional template. It is not itself knowledge.
_Avoid_: Type note, database schema

**Core property**:
A field with the same meaning on every Durable Object: `type`, `state`,
`tags`, and `aliases`.
_Avoid_: Type-owned field

**Type property**:
A custom property defined by one Type definition.
_Avoid_: Global property

**Knowledge lifecycle**:
The Vault-wide attention lifecycle stored in the `state` property: captured,
organized, and archived by default. Domain status such as task progress
belongs elsewhere.
_Avoid_: Task status, workflow status

**Relationship definition**:
A Vault-wide definition of a reference property between objects, with its
cardinality and inverse. Relationship values are readable links; inverse
edges are derived.
_Avoid_: Type-owned link field

**Saved view**:
A versioned declarative query with filters, grouping, sorting, and layout. It
stores no membership; results are always derived.
_Avoid_: Folder, collection with members

**Template**:
A starter for a new Durable Object, copied once. Later template changes never
alter objects created from it.
_Avoid_: Live template

**Taxonomy property**:
A property whose values form a configurable hierarchy for classification,
without implying subtype inheritance.
_Avoid_: Subtype, folder tree

**Tag**:
An open, flat, human-readable marker for lightweight cross-cutting discovery.
_Avoid_: Status, subtype, folder path

**Schema migration**:
An explicit, previewable rewrite offered when an Organization Model change
would reinterpret existing Markdown. Nothing is rewritten silently.
_Avoid_: Automatic schema upgrade

**Reconciliation**:
A repeatable diagnostic that reports Markdown that no longer fits the current
Organization Model and offers repairs without changing anything by itself.
_Avoid_: Background cleanup

**Contextual creation**:
Creating a note from a context that supplies its initial meaning: from a Type
assigns that Type; from Inbox or the global New Note action makes it
captured; from a Task List assigns that list.
_Avoid_: Folder-based creation

### Knowledge spaces

**Durable Object**:
A knowledge item with its own stable identity, lifecycle, relationships, and
revision history, stored as one Markdown file.
_Avoid_: File, block, database row

**Human Notes**:
The space of knowledge written or explicitly accepted by the user. AI may read
it under policy, but only the user writes it.
_Avoid_: AI Wiki, Source archive

**Human Note**:
One Durable Object in Human Notes. Its first heading is its title.
_Avoid_: Page, document, file

**Authored knowledge**:
Content the user wrote or accepted, always distinguishable from AI-generated
content even when displayed together.
_Avoid_: AI output

**Sources**:
The space of captured external material preserved as evidence with its
provenance. AI may read and interpret a Source but never rewrite it.
_Avoid_: Human Notes, Bookmark Page

**AI Knowledge**:
The AI-owned space where authorized agents freely create and maintain their
own objects using the same Markdown and Organization Model substrate. Users
read it; taking ownership of an item creates a separate Human Note.
_Avoid_: Human Notes, separate AI database

**AI write provenance**:
The system-recorded actor, channel, and time attached to everything written
into AI Knowledge.
_Avoid_: AI-chosen authorship

### Daily capture

**Daily Note**:
The Human Note for one local calendar date in the user's time zone. There is
at most one per date, and it has two regions: free-form Today entries and the
Memo stream.
_Avoid_: Journal file, dashboard

**Today**:
The default home: the current date's Daily Note together with Today context.
Visiting it does not create the Daily Note; the first write does.
_Avoid_: Daily dashboard, Inbox

**Today entry**:
Free-form Markdown written directly into a Daily Note outside the Memo stream.
It has no identity of its own.
_Avoid_: Memo, capture item

**Quick capture**:
The single input that accepts a thought or task without asking where it
belongs and appends it as a Memo to the current Daily Note.
_Avoid_: New note dialog, capture folder

**Memo**:
An identified, timestamped block in a Daily Note's Memo stream. It keeps its
identity when extracted into its own Human Note, and its original position
keeps a link to it.
_Avoid_: Today entry, Durable Object

**Memo comment**:
A later, separately timestamped addition to exactly one Memo, stored beside
that Memo in its original Daily Note regardless of when it was written.
Comments do not nest.
_Avoid_: Reply thread, new Memo

**Today context**:
Saved views shown beside Today, such as Tasks scheduled or overdue today and
captured Inbox notes. Results are never copied into the Daily Note.
_Avoid_: Copied task list, AI daily dashboard

**Inbox**:
The derived view of standalone Human Notes in the captured state, waiting to
be organized. Memos, Today entries, and Tasks are not Inbox items.
_Avoid_: Inbox folder, task inbox, Unlisted Tasks

### Tasks

**Task**:
An actionable item whose state lives in canonical Markdown where it was
written: either a Page Task or an Inline Task. There is no separate task
store.
_Avoid_: Checkbox, task database row

**Page Task**:
A Human Note that carries the `task` property, making the whole note the Task
and its body the Task's details.
_Avoid_: Linked task note

**Inline Task**:
An explicitly marked checkbox inside a Human Note or Memo. Its checkbox is the
completion state. An unmarked checkbox is only a local checklist item.
_Avoid_: Any checkbox, checklist item

**Task status**:
The state of a Task: open, paused, done, or canceled. Only recurring Tasks can
be paused. It is independent of the Knowledge lifecycle.
_Avoid_: `state`, lifecycle

**Task List**:
A Human Note of type `task-list` that Tasks reference to group action. A Task
belongs to at most one Task List, independent of folders, Type, and tags.
_Avoid_: Project folder, tag

**Unlisted Tasks**:
The derived view of open Tasks that reference no Task List. Assigning a list
changes only the Task's reference, never where it is written.
_Avoid_: Inbox, default Task List file

**Scheduled date**:
The local date on which the user plans to act on a Task.
_Avoid_: Due date

**Due date**:
The local date by which a one-off Task must be finished. Recurring Tasks have
no due date.
_Avoid_: Scheduled date

**Recurrence**:
The rule on a Page Task that schedules its next Occurrence, either on a fixed
calendar schedule or a fixed interval after the previous action. A recurring
Task persists across Occurrences.
_Avoid_: Repeating copy

**Occurrence**:
One round of a recurring Task, with a stable identity, an original date, and a
movable scheduled date. Only one Occurrence is current at a time; rounds
missed by acting late collapse into it.
_Avoid_: Instance, next task copy

**Occurrence record**:
The block appended to a recurring Task's body when its Occurrence is completed
or skipped, recording the outcome, the original and actual dates, and the
user's notes.
_Avoid_: Run, log line

### Sources and capture

**URL Capture**:
Turning a URL into a Source. A capable client starts it automatically when a
URL is added to a Memo; elsewhere it is explicit. The client retrieves and
extracts the page; the server validates and stores the untrusted result. If
retrieval fails, nothing is created and the plain URL remains for a later
retry.
_Avoid_: Server-guessed content, Bookmark Page

**Bookmarks**:
The derived view of Sources captured from Web pages. A bookmark is the Source
itself; the user's remarks stay in the Memo or Human Note that links to it.
_Avoid_: Bookmark Page, browser bookmark sync

**Source capture mode**:
How strongly a Source preserves its original: extracted Markdown only, or
extracted Markdown plus the original file. A citation claims only what the
preserved form supports.
_Avoid_: Unlabelled lossy copy

**Capture revision**:
One immutable preserved version of a Source. Recapturing changed content adds
a new revision under the same Source.
_Avoid_: Silent overwrite, one Source per refresh

### Retrieval

**Core retrieval**:
Finding authorized knowledge through text, properties, aliases, links, and
relationships, without any AI. Every result points to its object, revision,
and location.
_Avoid_: Vector search, cross-Vault search

**Lexical retrieval**:
The text-matching part of Core retrieval. It handles mixed Chinese and
English, ranks title and heading matches higher, and reliably finds
two-character Chinese words and identifiers such as model numbers.
Traditional and Simplified script forms match each other; regional vocabulary
does not. A query needs at least two Han characters or one complete
identifier.
_Avoid_: Semantic retrieval, single-character substring search

**Search index**:
The disposable per-Vault query model derived from canonical Markdown and
updated after every save. Deleting and rebuilding it never changes knowledge,
tasks, or citations.
_Avoid_: Canonical database, global search index

**Indexed block**:
One searchable passage, such as a section, Memo, Memo comment, or Occurrence
record, together with its owning object and source location.
_Avoid_: Arbitrary chunk, database-owned content

**Retrieval watermark**:
The latest Checkpoint commit fully reflected in the Search index. Owner search
does not depend on it; AI citations do, because they must point at committed
history.
_Avoid_: Search freshness, Object working revision

**Search refresh**:
Making current saved knowledge citable by AI before the next scheduled
checkpoint. Owner search never needs it.
_Avoid_: Manual Git commit

**Retrieval role**:
What a retrieved passage may be used for. Human Notes and Sources may be
evidence; AI Knowledge may guide discovery but never validates its own claims.
_Avoid_: AI Knowledge as primary evidence

**Graph navigation**:
Following links and relationships from known objects, including backlinks and
nearby objects, without changing text ranking.
_Avoid_: Graph-ranked search

**Semantic retrieval**:
An optional AI-era enhancement that finds conceptually related passages. It
is never required to search, rebuild, or recover a Vault.
_Avoid_: Core retrieval, required vector index

### AI

**Optional AI capability**:
A per-Vault capability that is off until configured. Every non-AI feature
works without it, and enabling it never expands AI authority over Human Notes
or Today.
_Avoid_: Core dependency, AI-first storage

**AI provider**:
An interchangeable external or self-hosted model service that the user
configures and authorizes for a Vault.
_Avoid_: Built-in AI backend

**AI Profile**:
The provider, model, and hard budget fixed for an AI Run before it starts.
_Avoid_: Model chosen by the agent, silent provider fallback

**AI Access Gateway**:
The only path through which AI or external Agents search, read, cite, or write
Vault knowledge. AI never receives direct access to the repository.
_Avoid_: Giving AI the repository path, shell access

**AI read policy**:
The Vault default for whether knowledge may be withheld from AI, read only by
local models, or sent to an authorized external provider. A single object may
be stricter, never looser.
_Avoid_: AI-controlled policy relaxation

**AI disclosure default**:
The explicit choice, made when an external AI provider is enabled, of the AI
read policy for ordinary objects. It is never inferred from adding
credentials.
_Avoid_: Silent provider consent

**Effective retrieval scope**:
The set of objects one AI Run may see, computed before ranking, snippets, or
link expansion.
_Avoid_: Post-retrieval redaction

**Retrieval source mode**:
Whether a cited answer may use only Vault knowledge or also Web search.
_Avoid_: Local execution mode

**AI Thread**:
A disposable multi-turn AI conversation in the Web client, whose durable
outcomes must be saved as Sources, AI Knowledge, or Change Proposals.
_Avoid_: Canonical chat object

**AI Run**:
The product-owned envelope of one AI turn or task, fixing its actor, Vault,
AI effect class, disclosure scope, and budget before it starts.
_Avoid_: Unbounded autonomous agent

**AI effect class**:
The maximum side effect of an AI Run: read-only, managed writes within AI
Knowledge and Sources, or proposals against human-owned content. A Run cannot
raise its own class.
_Avoid_: Dynamic privilege escalation

**Foreground Run**:
An AI Run started by a present user, able to ask questions and surface
conflicts immediately.
_Avoid_: Scheduled automation

**Scheduled Run**:
An unattended AI Run triggered by time or policy. Repository-writing Scheduled
Runs wait while any Change Proposal awaits a decision.
_Avoid_: Writes behind pending Review

**AI Run retry**:
Bounded restart of a failed AI Run from the same evidence. A failed Run never
advances its Processing cursor.
_Avoid_: Infinite retry

**AI Run cancellation**:
Stopping an AI Run and discarding its unpublished work. Once publication has
begun, recovery determines the complete outcome instead.
_Avoid_: Half-committed cancellation

**Vault AI prompt**:
A user-editable, versioned instruction that shapes one AI purpose within the
authority already granted to its Runs. It cannot expand that authority.
_Avoid_: Authority-bearing prompt

**Interactive AI suggestion**:
An AI-proposed insertion or edit inside the editor that enters Human Notes
only when the user accepts it.
_Avoid_: Autonomous inline edit

**Change Proposal**:
A reviewable, attributable set of changes that AI proposes against
human-owned content, with its base revisions, rationale, and citations.
Accepting revalidates and applies it whole; stale targets require a refresh.
_Avoid_: Automatic edit, background rewrite

**Review**:
The queue of decisions AI hands back only when an operation crosses into
human-owned content or has broad structural impact. Work inside AI Knowledge
takes effect without per-item approval.
_Avoid_: Approval for every AI note, notification feed

**Review decision**:
Accepting, rejecting, revising, or refreshing a whole Change Proposal. A
proposal is never partially or silently changed.
_Avoid_: Partial patch

**Review recent changes**:
A user-started AI conversation that summarizes what changed since its last
Processing cursor and may continue into exploration. Only results the user
asks to keep become durable.
_Avoid_: Scheduled daily report, Daily Summary object

**Knowledge answer**:
An AI answer grounded in authorized knowledge and linked to the passages that
support it.
_Avoid_: Generic chatbot response

**Answer grounding**:
The visible account of whether each part of a Knowledge answer came from the
Vault, the Web, or the model's own knowledge.
_Avoid_: Citation laundering

**Claim citation**:
A readable footnote linking a claim to the exact object, block, and revision
that supports it.
_Avoid_: Page-only bibliography, floating latest-source link

**Historical citation view**:
Showing a cited block as it existed at the cited revision, marked as
historical, with a link to the current object.
_Avoid_: Floating current-note citation

**Epistemic status**:
The visible distinction between a directly supported claim, an inferred
synthesis, and an unverified hypothesis.
_Avoid_: False certainty

**Memory pattern**:
A suggested AI Knowledge pattern for small durable facts written by an agent.
It is guidance, not a built-in object kind.
_Avoid_: Hard-coded Memory record

**Compiled Wiki pattern**:
A suggested AI Knowledge pattern for incrementally synthesizing concepts,
claims, and relationships across Human Notes and Sources, rather than
mirroring every note.
_Avoid_: Hard-coded Wiki page, rewrite of every note

**Knowledge compilation**:
The optional Scheduled Run that maintains the Compiled Wiki pattern from
evidence changed since its Processing cursor.
_Avoid_: Daily report, suggestion inbox

**Compilation invalidation**:
Re-evaluating AI Knowledge claims whose cited evidence changed or was
deleted.
_Avoid_: Append-only generated wiki

**Transient URL research**:
Fetching a URL inside an AI conversation without creating a Source. Anything
durable built on that page must first go through URL Capture.
_Avoid_: Automatic Source capture
