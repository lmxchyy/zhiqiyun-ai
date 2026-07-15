<template>
  <navigator
    class="native-page-back"
    aria-label="返回上一页"
    open-type="switchTab"
    :url="fallback"
    hover-class="none"
  >‹</navigator>
</template>

<script lang="ts">
import { defineComponent } from "vue";

export default defineComponent({
  name: "NativePageBack",
  props: {
    fallback: {
      type: String,
      default: "/pages/user/UserHomePage",
    },
  },
});
</script>

<style scoped>
:host {
  display: block;
  width: 34px;
  min-width: 34px;
  height: 34px;
  flex: 0 0 34px;
}

.native-page-back {
  display: grid;
  width: 34px;
  min-width: 34px;
  height: 34px;
  margin: 0;
  padding: 0;
  place-items: center;
  border: 1px solid #dfe5f2;
  border-radius: 8px;
  color: #475467;
  background: #fff;
  font-size: 22px;
  line-height: 1;
}

.native-page-back::after {
  display: none;
}
</style>
