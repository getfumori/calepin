<script setup lang="ts">
import { ref } from "vue";

import type { DailyMemo } from "./contracts/daily-note";

const props = defineProps<{ memos: DailyMemo[]; canCapture: boolean }>();
const emit = defineEmits<{
  capture: [text: string, task: boolean];
  comment: [memoId: string, text: string];
}>();

const draft = ref("");
const asTask = ref(false);
const commentDrafts = ref<Record<string, string>>({});
const openComment = ref<string>();

function timeOf(created: string): string {
  return created.slice(11, 16);
}

function submit(): void {
  const text = draft.value.trim();
  if (!text) return;
  emit("capture", text, asTask.value);
  draft.value = "";
  asTask.value = false;
}

function submitComment(memoId: string): void {
  const text = (commentDrafts.value[memoId] ?? "").trim();
  if (!text) return;
  emit("comment", memoId, text);
  commentDrafts.value[memoId] = "";
  openComment.value = undefined;
}

function plain(markdown: string): string {
  return markdown.replace(/\s*<!-- calepin:task \{.*?\} -->/g, "");
}
</script>

<template>
  <section class="memo-stream" aria-label="Memos">
    <h2 class="memo-heading">Memos</h2>
    <form v-if="props.canCapture" class="memo-capture" @submit.prevent="submit">
      <textarea
        v-model="draft"
        aria-label="Quick capture"
        placeholder="Capture a thought…"
        rows="2"
        @keydown.meta.enter.prevent="submit"
        @keydown.ctrl.enter.prevent="submit"
      ></textarea>
      <div class="memo-capture-actions">
        <label><input v-model="asTask" type="checkbox" /> As task</label>
        <button type="submit">Capture</button>
      </div>
    </form>
    <p v-if="props.memos.length === 0" class="memo-empty">No memos yet.</p>
    <article v-for="memo in props.memos" :key="memo.id" class="memo" :data-memo-id="memo.id">
      <time class="memo-time">{{ timeOf(memo.created) }}</time>
      <p class="memo-text">{{ plain(memo.markdown) }}</p>
      <div v-for="comment in memo.comments" :key="comment.id" class="memo-comment">
        <time class="memo-time">{{ comment.created.slice(0, 10) }} {{ timeOf(comment.created) }}</time>
        <p class="memo-text">{{ plain(comment.markdown) }}</p>
      </div>
      <form
        v-if="openComment === memo.id"
        class="memo-comment-form"
        @submit.prevent="submitComment(memo.id)"
      >
        <input
          v-model="commentDrafts[memo.id]"
          aria-label="Comment"
          placeholder="Add a comment…"
        />
        <button type="submit">Add</button>
      </form>
      <button v-else type="button" class="memo-reply" @click="openComment = memo.id">
        Comment
      </button>
    </article>
  </section>
</template>

<style scoped>
.memo-stream {
  margin-top: 48px;
  border-top: 1px solid var(--ivory-200);
  padding-top: 20px;
}
.memo-heading {
  font-size: 0.85rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--ink-500);
}
.memo-capture textarea {
  width: 100%;
  box-sizing: border-box;
  padding: 10px 12px;
  border: 1px solid var(--ivory-200);
  border-radius: 8px;
  font: inherit;
  background: var(--ivory-50);
  resize: vertical;
}
.memo-capture-actions {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 8px;
  color: var(--ink-700);
}
.memo-capture-actions button,
.memo-comment-form button {
  padding: 6px 14px;
  border: 1px solid var(--forest-700);
  border-radius: 6px;
  background: var(--forest-700);
  color: var(--ivory-50);
  cursor: pointer;
}
.memo-empty {
  color: var(--ink-500);
}
.memo {
  margin-top: 18px;
  padding: 12px 14px;
  border: 1px solid var(--ivory-200);
  border-radius: 8px;
}
.memo-time {
  font-size: 0.8rem;
  color: var(--ink-500);
}
.memo-text {
  margin: 4px 0 0;
  white-space: pre-wrap;
}
.memo-comment {
  margin: 10px 0 0 14px;
  padding-left: 10px;
  border-left: 2px solid var(--forest-100);
}
.memo-reply {
  margin-top: 8px;
  border: none;
  background: none;
  color: var(--forest-700);
  cursor: pointer;
  padding: 0;
}
.memo-comment-form {
  display: flex;
  gap: 8px;
  margin-top: 10px;
}
.memo-comment-form input {
  flex: 1;
  padding: 6px 10px;
  border: 1px solid var(--ivory-200);
  border-radius: 6px;
  font: inherit;
}
</style>
