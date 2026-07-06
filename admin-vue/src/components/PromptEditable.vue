<template>
  <div class="prompt-editable-shell">
    <div
      ref="editableRef"
      class="prompt-editable-input"
      role="textbox"
      aria-multiline="true"
      :data-placeholder="placeholder"
      :data-empty="modelValue ? 'false' : 'true'"
      contenteditable="true"
      spellcheck="false"
      @input="handleInput"
      @keydown="handleKeydown"
      @paste="handlePaste"
      @copy="handleCopy"
      @cut="handleCut"
      @focus="adjustHeight"
      @blur="adjustHeight"
    ></div>
    <button
      v-if="modelValue"
      class="prompt-editable-clear"
      type="button"
      aria-label="清空提示词"
      @click="clear"
    >
      ×
    </button>
  </div>
</template>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";

const props = withDefaults(defineProps<{
  modelValue: string;
  placeholder?: string;
  submitOnEnter?: boolean;
  minHeight?: number;
  maxHeight?: number;
}>(), {
  placeholder: "",
  submitOnEnter: false,
  minHeight: 48,
  maxHeight: 220
});

const emit = defineEmits<{
  "update:modelValue": [value: string];
  "pasteImages": [files: File[]];
  submit: [];
}>();

const editableRef = ref<HTMLDivElement | null>(null);
let composing = false;

function normalizeText(value: string) {
  return value.replace(/\r\n?/g, "\n");
}

function plainText(root: HTMLElement) {
  return normalizeText(root.innerText || "");
}

function selectionOffset(root: HTMLElement) {
  const selection = window.getSelection();
  if (!selection || selection.rangeCount === 0) return null;
  const range = selection.getRangeAt(0);
  if (!root.contains(range.startContainer)) return null;
  const before = range.cloneRange();
  before.selectNodeContents(root);
  before.setEnd(range.startContainer, range.startOffset);
  return normalizeText(before.toString()).length;
}

function restoreSelection(root: HTMLElement, offset: number) {
  const selection = window.getSelection();
  if (!selection) return;
  let remaining = Math.max(0, offset);
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  let lastText: Text | null = null;

  while (walker.nextNode()) {
    const node = walker.currentNode as Text;
    lastText = node;
    if (remaining <= node.length) {
      const range = document.createRange();
      range.setStart(node, remaining);
      range.collapse(true);
      selection.removeAllRanges();
      selection.addRange(range);
      return;
    }
    remaining -= node.length;
  }

  const range = document.createRange();
  if (lastText) {
    range.setStart(lastText, lastText.length);
  } else {
    range.selectNodeContents(root);
  }
  range.collapse(false);
  selection.removeAllRanges();
  selection.addRange(range);
}

function syncDomValue(value: string, preserveSelection = true) {
  const root = editableRef.value;
  if (!root) return;
  const nextValue = normalizeText(value);
  if (plainText(root) === nextValue) {
    adjustHeight();
    return;
  }

  const offset = preserveSelection ? selectionOffset(root) : null;
  root.innerText = nextValue;
  if (offset !== null && document.activeElement === root) restoreSelection(root, offset);
  adjustHeight();
}

function maxEditableHeight() {
  const viewportMax = typeof window === "undefined" ? props.maxHeight : Math.floor(window.innerHeight * 0.4);
  return Math.max(props.minHeight, Math.min(props.maxHeight, viewportMax));
}

function adjustHeight() {
  const root = editableRef.value;
  if (!root) return;
  const maxHeight = maxEditableHeight();
  root.style.height = "0px";
  const targetHeight = Math.min(Math.max(root.scrollHeight, props.minHeight), maxHeight);
  root.style.height = `${targetHeight}px`;
  root.style.overflowY = root.scrollHeight > maxHeight ? "auto" : "hidden";
}

function emitCurrentText() {
  const root = editableRef.value;
  if (!root) return;
  emit("update:modelValue", plainText(root));
  nextTick(adjustHeight);
}

function handleInput() {
  if (composing) return;
  emitCurrentText();
}

function insertTextAtSelection(text: string) {
  const selection = window.getSelection();
  if (!selection || selection.rangeCount === 0) return;
  selection.deleteFromDocument();
  const node = document.createTextNode(text);
  const range = selection.getRangeAt(0);
  range.insertNode(node);
  range.setStartAfter(node);
  range.collapse(true);
  selection.removeAllRanges();
  selection.addRange(range);
}

function handlePaste(event: ClipboardEvent) {
  const files = Array.from(event.clipboardData?.files || []);
  const imageFiles = files.filter((file) => file.type.startsWith("image/"));
  if (imageFiles.length) {
    event.preventDefault();
    emit("pasteImages", imageFiles);
    return;
  }

  const text = event.clipboardData?.getData("text/plain");
  if (text === undefined) return;
  event.preventDefault();
  insertTextAtSelection(normalizeText(text));
  emitCurrentText();
}

function handleCopy(event: ClipboardEvent) {
  const selection = window.getSelection();
  const selectedText = selection && selection.rangeCount > 0 ? selection.toString() : "";
  event.preventDefault();
  event.clipboardData?.setData("text/plain", normalizeText(selectedText || props.modelValue));
}

function handleCut(event: ClipboardEvent) {
  handleCopy(event);
  window.getSelection()?.deleteFromDocument();
  emitCurrentText();
}

function handleKeydown(event: KeyboardEvent) {
  if ((event.ctrlKey || event.metaKey) && event.key === "Enter") {
    event.preventDefault();
    emit("submit");
    return;
  }
  if (event.key === "Enter" && props.submitOnEnter && !event.shiftKey) {
    event.preventDefault();
    emit("submit");
  }
}

function focus() {
  const root = editableRef.value;
  if (!root) return;
  root.focus();
  restoreSelection(root, plainText(root).length);
}

function clear() {
  emit("update:modelValue", "");
  nextTick(() => {
    syncDomValue("", false);
    focus();
  });
}

function handleCompositionStart() {
  composing = true;
}

function handleCompositionEnd() {
  composing = false;
  emitCurrentText();
}

onMounted(() => {
  const root = editableRef.value;
  if (!root) return;
  root.addEventListener("compositionstart", handleCompositionStart);
  root.addEventListener("compositionend", handleCompositionEnd);
  syncDomValue(props.modelValue, false);
  window.addEventListener("resize", adjustHeight);
});

onBeforeUnmount(() => {
  const root = editableRef.value;
  root?.removeEventListener("compositionstart", handleCompositionStart);
  root?.removeEventListener("compositionend", handleCompositionEnd);
  window.removeEventListener("resize", adjustHeight);
});

watch(() => props.modelValue, (value) => {
  syncDomValue(value);
});

defineExpose({ focus, adjustHeight });
</script>
