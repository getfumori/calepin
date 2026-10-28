<script setup lang="ts">
import { computed, onMounted, ref } from "vue";

import { typeResultResponseSchema } from "./contracts/organization-model";
import {
  listTitle,
  type OccurrenceRecord,
  type PageTask,
  type Recurrence
} from "./contracts/tasks";

const props = defineProps<{
  task: PageTask | null;
  occurrences: OccurrenceRecord[];
}>();
const emit = defineEmits<{
  command: [action: string, body: Record<string, unknown>];
}>();

const lists = ref<string[]>([]);
const today = ref(new Date().toISOString().slice(0, 10));
const roundNotes = ref("");
const making = ref(false);
const firstDate = ref(today.value);
const repeat = ref<"none" | Recurrence["frequency"]>("none");
const interval = ref(1);
const basis = ref<Recurrence["basis"]>("schedule");
const newList = ref("");

const weekdayNames = ["sun", "mon", "tue", "wed", "thu", "fri", "sat"] as const;

onMounted(async () => {
  const [listResponse, todayResponse] = await Promise.all([
    fetch("/api/v1/types/task-list", { cache: "no-store" }),
    fetch("/api/v1/today", { cache: "no-store" })
  ]);
  if (listResponse.ok) {
    lists.value = typeResultResponseSchema
      .parse(await listResponse.json())
      .items.map((item) => item.title);
  }
  if (todayResponse.ok) {
    today.value = ((await todayResponse.json()) as { date: string }).date;
    firstDate.value = today.value;
  }
});

const recurring = computed(() => props.task?.recurrence !== undefined);
const scheduled = computed(
  () => props.task?.occurrence?.scheduled ?? props.task?.scheduled
);
const overdue = computed(
  () =>
    props.task?.status === "open" &&
    scheduled.value !== undefined &&
    scheduled.value < today.value
);
const history = computed(() => [...props.occurrences].reverse());

function ruleLabel(rule: Recurrence): string {
  const unit = { daily: "day", weekly: "week", monthly: "month" }[rule.frequency];
  const every = rule.interval === 1 ? `Every ${unit}` : `Every ${rule.interval} ${unit}s`;
  const names = { sun: "Sunday", mon: "Monday", tue: "Tuesday", wed: "Wednesday", thu: "Thursday", fri: "Friday", sat: "Saturday" };
  const days = rule.weekdays?.length ? ` on ${rule.weekdays.map((day) => names[day]).join(", ")}` : "";
  return `${every}${days}${rule.basis === "completion" ? " after completion" : ""}`;
}

function make(): void {
  const body: Record<string, unknown> = { scheduled: firstDate.value };
  if (newList.value) body.list = `[[${newList.value}]]`;
  if (repeat.value !== "none") {
    const rule: Recurrence = {
      basis: basis.value,
      frequency: repeat.value,
      interval: Math.max(1, interval.value)
    };
    if (repeat.value === "weekly" && basis.value === "schedule") {
      rule.weekdays = [weekdayNames[new Date(`${firstDate.value}T12:00:00Z`).getUTCDay()]!];
    }
    body.recurrence = rule;
  }
  emit("command", "make", body);
  making.value = false;
}

function round(action: "complete" | "skip"): void {
  emit("command", action, {
    expectedOccurrence: props.task?.occurrence?.id,
    notes: roundNotes.value
  });
  roundNotes.value = "";
}

function setList(event: Event): void {
  const title = (event.target as HTMLSelectElement).value;
  emit("command", "update", { list: title ? `[[${title}]]` : "" });
}
</script>

<template>
  <section class="task-panel" aria-label="Task">
    <template v-if="!props.task">
      <button v-if="!making" type="button" class="task-link" @click="making = true">
        Make this a task
      </button>
      <form v-else class="task-make" @submit.prevent="make">
        <label>Date <input v-model="firstDate" type="date" required /></label>
        <label>
          Repeat
          <select v-model="repeat" aria-label="Repeat">
            <option value="none">Does not repeat</option>
            <option value="daily">Daily</option>
            <option value="weekly">Weekly</option>
            <option value="monthly">Monthly</option>
          </select>
        </label>
        <template v-if="repeat !== 'none'">
          <label>Every <input v-model.number="interval" type="number" min="1" /></label>
          <label>
            <select v-model="basis" aria-label="Repeat basis">
              <option value="schedule">On the calendar</option>
              <option value="completion">After completion</option>
            </select>
          </label>
        </template>
        <label>
          List
          <select v-model="newList" aria-label="Task List">
            <option value="">Unlisted</option>
            <option v-for="list in lists" :key="list" :value="list">{{ list }}</option>
          </select>
        </label>
        <button type="submit">Create task</button>
        <button type="button" class="task-link" @click="making = false">Cancel</button>
      </form>
    </template>

    <template v-else>
      <div class="task-summary">
        <span class="task-status" :data-status="props.task.status">{{ props.task.status }}</span>
        <span v-if="props.task.recurrence">{{ ruleLabel(props.task.recurrence) }}</span>
        <span v-if="scheduled" :class="{ overdue }">
          {{ recurring ? "Next round" : "Scheduled" }} {{ scheduled }}
        </span>
        <span v-if="props.task.due">Due {{ props.task.due }}</span>
        <select
          :value="listTitle(props.task.list)"
          aria-label="Task List"
          @change="setList"
        >
          <option value="">Unlisted</option>
          <option v-for="list in lists" :key="list" :value="list">{{ list }}</option>
        </select>
      </div>

      <div v-if="recurring && props.task.status === 'open'" class="task-round">
        <textarea
          v-model="roundNotes"
          aria-label="Notes for this round"
          placeholder="What happened this round?"
          rows="3"
        ></textarea>
        <div class="task-actions">
          <button type="button" @click="round('complete')">Complete round</button>
          <button type="button" class="secondary" @click="round('skip')">Skip</button>
          <button type="button" class="task-link" @click="emit('command', 'pause', {})">Pause</button>
        </div>
      </div>
      <div v-else class="task-actions">
        <button
          v-if="!recurring && props.task.status === 'open'"
          type="button"
          @click="emit('command', 'complete', {})"
        >
          Complete
        </button>
        <button
          v-if="props.task.status === 'paused'"
          type="button"
          @click="emit('command', 'resume', {})"
        >
          Resume
        </button>
        <button
          v-if="!recurring && props.task.status !== 'open'"
          type="button"
          class="secondary"
          @click="emit('command', 'reopen', {})"
        >
          Reopen
        </button>
      </div>

      <div v-if="history.length" class="task-history">
        <div class="task-history-heading">
          <h2>History</h2>
          <button
            v-if="recurring && props.task.occurrence"
            type="button"
            class="task-link"
            @click="emit('command', 'undo', { expectedOccurrence: props.task.occurrence.id })"
          >
            Undo last
          </button>
        </div>
        <article v-for="record in history" :key="record.id" class="task-record">
          <header>
            <strong>{{ record.at.slice(0, 10) }}</strong>
            <span :data-outcome="record.outcome">{{ record.outcome }}</span>
            <small v-if="record.at.slice(0, 10) !== record.original">for {{ record.original }}</small>
          </header>
          <p>{{ record.markdown.replace(/^### .*\n*/, "") }}</p>
        </article>
      </div>
    </template>
  </section>
</template>

<style scoped>
.task-panel {
  margin: 18px 0 8px;
  padding: 14px 16px;
  border: 1px solid var(--ivory-200);
  border-radius: 10px;
  background: var(--ivory-50);
}
.task-summary,
.task-actions,
.task-make {
  display: flex;
  flex-wrap: wrap;
  gap: 10px 14px;
  align-items: center;
  color: var(--ink-700);
}
.task-make label {
  display: flex;
  gap: 6px;
  align-items: center;
}
.task-make input[type="number"] {
  width: 4em;
}
.task-status {
  padding: 2px 8px;
  border-radius: 999px;
  background: var(--forest-100);
  color: var(--forest-800);
  font-size: 0.8rem;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}
.overdue {
  color: #a3412c;
  font-weight: 600;
}
.task-round textarea {
  width: 100%;
  box-sizing: border-box;
  margin: 12px 0 8px;
  padding: 8px 10px;
  border: 1px solid var(--ivory-200);
  border-radius: 8px;
  font: inherit;
}
button {
  padding: 6px 14px;
  border: 1px solid var(--forest-700);
  border-radius: 6px;
  background: var(--forest-700);
  color: var(--ivory-50);
  cursor: pointer;
}
button.secondary {
  background: transparent;
  color: var(--forest-700);
}
button.task-link {
  border: none;
  background: none;
  padding: 0;
  color: var(--forest-700);
}
.task-history {
  margin-top: 16px;
}
.task-history-heading {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
}
.task-history h2 {
  font-size: 0.85rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--ink-500);
}
.task-record {
  margin-top: 10px;
  padding-left: 10px;
  border-left: 2px solid var(--forest-100);
}
.task-record header {
  display: flex;
  gap: 8px;
  align-items: baseline;
}
.task-record [data-outcome="skipped"] {
  color: var(--ink-500);
}
.task-record p {
  margin: 4px 0 0;
  white-space: pre-wrap;
}
</style>
