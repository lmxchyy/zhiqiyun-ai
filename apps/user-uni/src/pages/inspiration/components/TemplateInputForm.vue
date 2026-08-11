<template>
  <view class="template-form">
    <view v-for="group in groups" :key="group.key" class="form-section">
      <view class="section-head">
        <text>{{ group.label }}</text>
        <small v-if="group.key === 'advanced'">可选</small>
      </view>
      <view v-for="input in group.inputs" :key="input.key" class="field">
        <view class="field-head">
          <text>{{ input.label }}<small v-if="input.required"> *</small></text>
          <text v-if="input.helpText" class="field-help">{{ input.helpText }}</text>
        </view>

        <input
          v-if="control(input) === 'TEXT'"
          class="text-control"
          :value="stringValue(values[input.key])"
          :placeholder="input.placeholder"
          :maxlength="input.validation?.maxLength || 500"
          @input="setInputValue(input.key, $event)"
        />
        <textarea
          v-else-if="control(input) === 'TEXTAREA'"
          class="textarea-control"
          :value="stringValue(values[input.key])"
          :placeholder="input.placeholder"
          :maxlength="input.validation?.maxLength || 2000"
          auto-height
          @input="setInputValue(input.key, $event)"
        />
        <picker
          v-else-if="control(input) === 'SELECT'"
          mode="selector"
          :range="input.options || []"
          range-key="label"
          :value="selectedIndex(input)"
          @change="selectOption(input, $event)"
        >
          <view :class="['select-control', { empty: selectedIndex(input) < 0 }]">
            <text>{{ selectedLabel(input) || input.placeholder || `请选择${input.label}` }}</text><text>⌄</text>
          </view>
        </picker>
        <view v-else-if="control(input) === 'MULTI_SELECT'" class="choice-grid multi">
          <button
            v-for="option in input.options || []"
            :key="String(option.value)"
            :class="{ active: selectedValues(input).some(value => sameValue(value, option.value)) }"
            @click="toggleOption(input.key, option.value)"
          >{{ option.label }}</button>
        </view>
        <view v-else-if="control(input) === 'SEGMENTED'" class="choice-grid segmented">
          <button
            v-for="option in input.options || []"
            :key="String(option.value)"
            :class="{ active: sameValue(values[input.key], option.value) }"
            @click="setValue(input.key, option.value)"
          >{{ option.label }}</button>
        </view>
        <switch
          v-else-if="control(input) === 'BOOLEAN'"
          :checked="values[input.key] === true"
          color="#5368e8"
          @change="setBoolean(input.key, $event)"
        />
        <input
          v-else-if="control(input) === 'NUMBER'"
          class="text-control number-control"
          type="number"
          :value="numberText(values[input.key])"
          :placeholder="input.placeholder"
          @input="setNumber(input.key, $event)"
        />
        <view v-else-if="control(input) === 'SLIDER'" class="slider-control">
          <slider
            :value="numberValue(values[input.key], input.validation?.min || 0)"
            :min="input.validation?.min || 0"
            :max="input.validation?.max || 100"
            :step="1"
            active-color="#5368e8"
            background-color="#e7eaf1"
            block-size="18"
            @change="setSlider(input.key, $event)"
          />
          <text>{{ numberValue(values[input.key], input.validation?.min || 0) }}</text>
        </view>
        <TemplateAssetUpload
          v-else-if="control(input) === 'ASSET_UPLOAD'"
          :input="input"
          :model-value="assets[input.key] || []"
          :error="errors[input.key]"
          @update:model-value="setAssets(input.key, $event)"
        />
        <text v-if="errors[input.key] && control(input) !== 'ASSET_UPLOAD'" class="field-error">{{ errors[input.key] }}</text>
      </view>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { groupTemplateInputs, templateInputControl } from "../../../features/inspiration/contracts";
import type { PublicTemplateInput, TemplateAssetValues, TemplateUploadedAsset } from "../../../features/inspiration/types";
import TemplateAssetUpload from "./TemplateAssetUpload.vue";

const props = defineProps<{
  inputs: PublicTemplateInput[];
  values: Record<string, unknown>;
  assets: TemplateAssetValues;
  errors: Record<string, string>;
}>();
const emit = defineEmits<{
  "update:values": [value: Record<string, unknown>];
  "update:assets": [value: TemplateAssetValues];
}>();

const groups = computed(() => groupTemplateInputs(props.inputs, props.values));
const control = templateInputControl;
const stringValue = (value: unknown) => typeof value === "string" ? value : "";
const numberText = (value: unknown) => typeof value === "number" ? String(value) : "";
const numberValue = (value: unknown, fallback: number) => Number.isFinite(Number(value)) ? Number(value) : fallback;
const sameValue = (left: unknown, right: unknown) => JSON.stringify(left) === JSON.stringify(right);

function eventValue(event: unknown) {
  return (event as { detail?: { value?: unknown } })?.detail?.value;
}

function setValue(key: string, value: unknown) {
  emit("update:values", { ...props.values, [key]: value });
}

function setInputValue(key: string, event: unknown) {
  setValue(key, String(eventValue(event) ?? ""));
}

function setBoolean(key: string, event: unknown) {
  setValue(key, eventValue(event) === true);
}

function setNumber(key: string, event: unknown) {
  const raw = String(eventValue(event) ?? "");
  setValue(key, raw === "" ? undefined : Number(raw));
}

function setSlider(key: string, event: unknown) {
  setValue(key, Number(eventValue(event)));
}

function selectedValues(input: PublicTemplateInput) {
  return Array.isArray(props.values[input.key]) ? props.values[input.key] as unknown[] : [];
}

function toggleOption(key: string, option: unknown) {
  const current = Array.isArray(props.values[key]) ? props.values[key] as unknown[] : [];
  setValue(key, current.some(value => sameValue(value, option))
    ? current.filter(value => !sameValue(value, option))
    : [...current, option]);
}

function selectedIndex(input: PublicTemplateInput) {
  return (input.options || []).findIndex(option => sameValue(option.value, props.values[input.key]));
}

function selectedLabel(input: PublicTemplateInput) {
  return (input.options || [])[selectedIndex(input)]?.label || "";
}

function selectOption(input: PublicTemplateInput, event: unknown) {
  const index = Number(eventValue(event));
  const option = (input.options || [])[index];
  if (option) setValue(input.key, option.value);
}

function setAssets(key: string, value: TemplateUploadedAsset[]) {
  emit("update:assets", { ...props.assets, [key]: value });
}
</script>

<style scoped>
.template-form{display:flex;flex-direction:column;gap:12px}.form-section{padding:17px;border:1px solid #e8eaf1;border-radius:13px;background:#fff}.section-head{display:flex;align-items:center;justify-content:space-between;margin-bottom:4px}.section-head>text{font-size:15px;font-weight:700}.section-head small{padding:3px 7px;border-radius:9px;color:#858da0;background:#f2f4f8;font-size:9px}.field{padding-top:16px}.field-head{display:flex;align-items:flex-start;justify-content:space-between;margin-bottom:8px;gap:12px}.field-head>text:first-child{color:#30384c;font-size:12px;font-weight:650}.field-head>text:first-child small{color:#e14e61}.field-help{max-width:55%;color:#9199aa;font-size:9px;line-height:15px;text-align:right}.text-control,.textarea-control,.select-control{box-sizing:border-box;width:100%;border:1px solid #e2e5ec;border-radius:10px;color:#2e374a;background:#f8f9fb;font-size:12px}.text-control{height:43px;padding:0 12px}.textarea-control{min-height:82px;padding:11px 12px;line-height:20px}.select-control{display:flex;height:43px;align-items:center;justify-content:space-between;padding:0 12px}.select-control.empty{color:#a2a9b7}.choice-grid{display:flex;flex-wrap:wrap;gap:8px}.choice-grid button{width:auto;min-width:66px;height:36px;padding:0 13px;border:1px solid #e1e5ec;border-radius:10px;color:#687286;background:#f8f9fb;font-size:11px}.choice-grid button.active{border-color:#8494f0;color:#435bd7;background:#eef1ff}.segmented{display:grid;grid-template-columns:repeat(auto-fit,minmax(70px,1fr));gap:5px;padding:4px;border-radius:11px;background:#f1f3f7}.segmented button{width:100%;border:0;background:transparent}.segmented button.active{color:#364fc7;background:#fff;box-shadow:0 2px 8px rgba(44,56,94,.09)}.slider-control{display:grid;grid-template-columns:1fr 36px;align-items:center;gap:8px}.slider-control slider{margin:0}.slider-control>text{text-align:right;color:#5368e8;font-size:12px;font-weight:650}.field-error{display:block;margin-top:7px;color:#d94d5e;font-size:10px}button:after{border:0}
</style>
