<script setup lang="ts">
import { ref, onMounted } from "vue";
import { ApiReference } from "@scalar/api-reference";
import "@scalar/api-reference/style.css";
import { ArrowLeft } from "lucide-vue-next";
import { useAuthStore } from "@/stores/auth";
import { apiUrl } from "@/lib/api";

const auth = useAuthStore();
const topbar = ref<HTMLElement>();
const headerHeight = ref("0px");
const spec = ref<unknown>(null);
const loadError = ref(false);

onMounted(async () => {
  if (topbar.value) {
    headerHeight.value = `${topbar.value.offsetHeight}px`;
  }

  const headers: Record<string, string> = {};
  const token = auth.token ?? auth.apiKeyToken;
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }

  try {
    const res = await fetch(apiUrl("/api/openapi.json"), { headers });
    if (res.ok) {
      spec.value = await res.json();
    } else {
      loadError.value = true;
    }
  } catch {
    loadError.value = true;
  }
});
</script>

<template>
  <div class="docs-page" :style="{ '--topbar-height': headerHeight }">
    <header ref="topbar" class="docs-topbar">
      <router-link to="/" class="docs-back-link">
        <ArrowLeft class="docs-back-icon" />
        <span class="docs-title">OpenPosterDB</span>
      </router-link>
      <span class="docs-subtitle">API Reference</span>
    </header>
    <ApiReference v-if="spec" :configuration="{
      content: spec,
      hideClientButton: true,
      showDeveloperTools: 'never',
      mcp: { disabled: true },
      agent: { disabled: true },
      forceDarkModeState: 'light',
      hideDarkModeToggle: true,
      defaultOpenAllTags: true,
      withDefaultFonts: false,
    }" />
    <p v-else-if="loadError" class="docs-error" role="alert">The API reference could not be loaded.</p>
  </div>
</template>

<style scoped>
.docs-error {
  padding: 2rem 1rem;
  text-align: center;
}
.docs-page :deep(.scalar-app) {
  --scalar-custom-header-height: var(--topbar-height);
}

.docs-topbar {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.625rem 1rem;
  border-bottom: 1px solid #e5e5e5;
  background: #fff;
  position: sticky;
  top: 0;
  z-index: 100;
}

.docs-back-link {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  text-decoration: none;
  color: #0a0a0a;
  font-weight: 600;
  font-size: 0.875rem;
}

.docs-back-link:hover {
  color: #525252;
}

.docs-back-icon {
  width: 1rem;
  height: 1rem;
}

.docs-subtitle {
  color: #a3a3a3;
  font-size: 0.875rem;
}
</style>
