<script setup lang="ts">
import { computed, onMounted, ref } from "vue";

import { typeResultResponseSchema } from "./contracts/organization-model";
import {
  listTitle,
  taskListResponseSchema,
  type TaskView
} from "./contracts/tasks";
import PrimarySidebar from "./PrimarySidebar.vue";

type Filter =
  | { kind: "today" }
  | { kind: "unlisted" }
  | { kind: "all" }
  | { kind: "done" }
  | { kind: "list"; title: string };

const params = new URLSearchParams(window.location.search);
const initialList = params.get("list");
const filter = ref<Filter>(
  initialList ? { kind: "list", title: initialList } : { kind: (params.get("filter") as "today") ?? "today" }
);
const vaultName = ref<string>();
const today = ref("");
const open = ref<TaskView[]>([]);
const done = ref<TaskView[]>([]);
const lists = ref<string[]>([]);
const error = ref<string>();
const draft = ref("");
const loaded = ref(false);

async function load(): Promise<void> {
  try {
    const [todayResponse, openResponse, doneResponse, listResponse] = await Promise.all([
      fetch("/api/v1/today", { cache: "no-store" }),
      fetch("/api/v1/tasks", { cache: "no-store" }),
      fetch("/api/v1/tasks?status=done", { cache: "no-store" }),
      fetch("/api/v1/types/task-list", { cache: "no-store" })
    ]);
    const todayNote = (await todayResponse.json()) as { date: string; vault: { name: string } };
    today.value = todayNote.date;
    vaultName.value = todayNote.vault.name;
    open.value = taskListResponseSchema.parse(await openResponse.json());
    done.value = taskListResponseSchema.parse(await doneResponse.json());
    lists.value = typeResultResponseSchema
      .parse(await listResponse.json())
      .items.map((item) => item.title);
    loaded.value = true;
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : "Tasks could not be loaded.";
  }
}

onMounted(load);

const dueToday = (task: TaskView) =>
  task.overdue ||
  (task.scheduled !== undefined && task.scheduled <= today.value) ||
  (task.due !== undefined && task.due <= today.value);

function tasksFor(f: Filter): TaskView[] {
  switch (f.kind) {
    case "today":
      return open.value.filter(dueToday);
    case "unlisted":
      return open.value.filter((task) => !task.list);
    case "done":
      return done.value;
    case "list":
      return open.value.filter((task) => listTitle(task.list).toLowerCase() === f.title.toLowerCase());
    default:
      return open.value;
  }
}

const visible = computed(() => tasksFor(filter.value));

const heading = computed(() => {
  const f = filter.value;
  return f.kind === "list"
    ? f.title
    : { today: "Today", unlisted: "Unlisted", all: "All open", done: "Done" }[f.kind];
});

function count(f: Filter): number {
  return tasksFor(f).length;
}

function select(f: Filter): void {
  filter.value = f;
  const query = f.kind === "list" ? `?list=${encodeURIComponent(f.title)}` : `?filter=${f.kind}`;
  window.history.replaceState(null, "", `/tasks${query}`);
}

function isActive(f: Filter): boolean {
  const current = filter.value;
  return current.kind === f.kind && (f.kind !== "list" || (current.kind === "list" && current.title === f.title));
}

async function command(task: TaskView, action: string, body: Record<string, unknown> = {}): Promise<void> {
  error.value = undefined;
  const response = await fetch(`/api/v1/tasks/${task.id}/${action}`, {
    method: "POST",
    headers: { "Content-Type": "application/json", "Idempotency-Key": crypto.randomUUID() },
    body: JSON.stringify(body)
  });
  if (!response.ok) {
    const detail = (await response.json().catch(() => undefined)) as { message?: string } | undefined;
    error.value = detail?.message ?? `Task update failed (${response.status})`;
  }
  await load();
}

function toggle(task: TaskView): void {
  if (task.status === "done") {
    void command(task, "reopen");
  } else if (task.recurring) {
    void command(task, "complete", { expectedOccurrence: task.occurrence?.id });
  } else {
    void command(task, "complete");
  }
}

function reschedule(task: TaskView, event: Event): void {
  void command(task, "update", { scheduled: (event.target as HTMLInputElement).value });
}

function setDue(task: TaskView, event: Event): void {
  void command(task, "update", { due: (event.target as HTMLInputElement).value });
}

function move(task: TaskView, event: Event): void {
  const title = (event.target as HTMLSelectElement).value;
  void command(task, "update", { list: title ? `[[${title}]]` : "" });
}

async function add(): Promise<void> {
  const text = draft.value.trim();
  if (!text) return;
  const f = filter.value;
  const response = await fetch("/api/v1/capture", {
    method: "POST",
    headers: { "Content-Type": "application/json", "Idempotency-Key": crypto.randomUUID() },
    body: JSON.stringify({ text, task: true, list: f.kind === "list" ? `[[${f.title}]]` : "" })
  });
  if (!response.ok) {
    error.value = `The task could not be added (${response.status})`;
    return;
  }
  draft.value = "";
  await load();
}

function navigate(destination: string): void {
  window.location.assign(destination);
}

async function createNote(): Promise<void> {
  const response = await fetch("/api/v1/notes", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ context: "global" })
  });
  if (response.ok) {
    window.location.assign(`/notes/${((await response.json()) as { id: string }).id}`);
  }
}
</script>

<template>
  <main class="app-shell" :data-app-ready="loaded || error !== undefined">
    <PrimarySidebar :vault-name="vaultName" active="tasks" @navigate="navigate" @create="createNote" />

    <section class="context-zone" data-zone="context" aria-label="Task filters">
      <header class="zone-header">
        <p class="eyebrow">Tasks</p>
        <h2>{{ heading }}</h2>
      </header>
      <nav class="task-filters" aria-label="Task filters">
        <button
          v-for="f in [{ kind: 'today' }, { kind: 'unlisted' }, { kind: 'all' }] as Filter[]"
          :key="f.kind"
          type="button"
          :class="{ active: isActive(f) }"
          @click="select(f)"
        >
          <span>{{ { today: "Today", unlisted: "Unlisted", all: "All open" }[f.kind as "today"] }}</span>
          <small>{{ count(f) }}</small>
        </button>
        <p class="task-filter-group">Task Lists</p>
        <button
          v-for="title in lists"
          :key="title"
          type="button"
          :class="{ active: isActive({ kind: 'list', title }) }"
          @click="select({ kind: 'list', title })"
        >
          <span>{{ title }}</span>
          <small>{{ count({ kind: "list", title }) }}</small>
        </button>
        <p v-if="lists.length === 0" class="task-filter-hint">
          Create a note of type Task List to group tasks.
        </p>
        <button type="button" :class="{ active: isActive({ kind: 'done' }) }" @click="select({ kind: 'done' })">
          <span>Done</span>
          <small>{{ done.length }}</small>
        </button>
      </nav>
    </section>

    <article class="document-zone" data-zone="document">
      <section class="document task-document">
        <p class="eyebrow">Tasks</p>
        <h1>{{ heading }}</h1>
        <form v-if="filter.kind !== 'done'" class="task-add" @submit.prevent="add">
          <input v-model="draft" aria-label="New task" placeholder="Add a task…" />
          <button type="submit">Add</button>
        </form>
        <p v-if="error" class="save-error" role="alert">{{ error }}</p>
        <p v-if="loaded && visible.length === 0" class="task-empty">Nothing here.</p>
        <ul class="task-rows">
          <li v-for="task in visible" :key="task.id" class="task-row" :data-task-id="task.id">
            <input
              type="checkbox"
              :checked="task.status === 'done'"
              :aria-label="`Complete ${task.title}`"
              @change="toggle(task)"
            />
            <div class="task-main">
              <a v-if="task.kind === 'page'" :href="task.source.url" class="task-title">{{ task.title }}</a>
              <span v-else class="task-title">{{ task.title }}</span>
              <div class="task-meta">
                <span v-if="task.recurring" class="task-badge">Repeats</span>
                <span v-if="task.overdue" class="task-overdue">Overdue</span>
                <span v-if="task.due && task.status === 'done'">Due {{ task.due }}</span>
                <a v-if="task.kind === 'inline'" :href="task.source.url">{{ task.source.title }}</a>
              </div>
            </div>
            <input
              v-if="task.status !== 'done'"
              type="date"
              :value="task.scheduled ?? ''"
              :aria-label="`Schedule ${task.title}`"
              @change="reschedule(task, $event)"
            />
            <input
              v-if="task.status !== 'done' && !task.recurring"
              type="date"
              :value="task.due ?? ''"
              :aria-label="`Due date for ${task.title}`"
              title="Due date"
              @change="setDue(task, $event)"
            />
            <select
              v-if="task.status !== 'done'"
              :value="listTitle(task.list)"
              :aria-label="`List for ${task.title}`"
              @change="move(task, $event)"
            >
              <option value="">Unlisted</option>
              <option v-for="title in lists" :key="title" :value="title">{{ title }}</option>
            </select>
            <button
              v-if="task.recurring && task.status === 'open'"
              type="button"
              class="task-skip"
              @click="command(task, 'skip', { expectedOccurrence: task.occurrence?.id })"
            >
              Skip
            </button>
          </li>
        </ul>
      </section>
    </article>
  </main>
</template>

<style scoped>
.task-filters {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin-top: 16px;
}
.task-filters button {
  display: flex;
  justify-content: space-between;
  padding: 8px 10px;
  border: none;
  border-radius: 6px;
  background: none;
  color: var(--ink-700);
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.task-filters button.active {
  background: var(--forest-100);
  color: var(--forest-800);
}
.task-filter-group {
  margin: 16px 10px 4px;
  font-size: 0.75rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--ink-500);
}
.task-filter-hint,
.task-empty {
  margin: 4px 10px;
  color: var(--ink-500);
  font-size: 0.9rem;
}
.task-add {
  display: flex;
  gap: 8px;
  margin: 20px 0;
}
.task-add input {
  flex: 1;
  padding: 8px 12px;
  border: 1px solid var(--ivory-200);
  border-radius: 8px;
  font: inherit;
}
.task-add button,
.task-skip {
  padding: 6px 14px;
  border: 1px solid var(--forest-700);
  border-radius: 6px;
  background: var(--forest-700);
  color: var(--ivory-50);
  cursor: pointer;
}
.task-skip {
  background: transparent;
  color: var(--forest-700);
}
.task-rows {
  list-style: none;
  padding: 0;
}
.task-row {
  display: flex;
  gap: 12px;
  align-items: center;
  padding: 10px 0;
  border-bottom: 1px solid var(--ivory-200);
}
.task-main {
  flex: 1;
}
.task-title {
  color: var(--forest-950);
}
.task-meta {
  display: flex;
  gap: 10px;
  font-size: 0.8rem;
  color: var(--ink-500);
}
.task-badge {
  color: var(--forest-700);
}
.task-overdue {
  color: #a3412c;
  font-weight: 600;
}
</style>
