<script setup lang="ts">
import { useRoute } from 'vue-router'
import {
  SidebarProvider,
  SidebarInset,
  SidebarTrigger,
} from '@/components/ui/sidebar'
import { Separator } from '@/components/ui/separator'
import { Button } from '@/components/ui/button'
import { BookOpen } from 'lucide-vue-next'
import AppSidebar from '@/components/AppSidebar.vue'
import { useAuthStore } from '@/stores/auth'
import { useIdleLogout } from '@/composables/useIdleLogout'

const route = useRoute()
const auth = useAuthStore()
useIdleLogout()
</script>

<template>
  <SidebarProvider>
    <AppSidebar />
    <SidebarInset>
      <header class="flex h-12 shrink-0 items-center gap-2 border-b px-4 pr-6">
        <SidebarTrigger class="-ml-1" />
        <Separator orientation="vertical" class="mr-2 !h-4" />
        <span class="text-sm font-medium">{{ route.meta.title || route.name }}</span>
        <Button v-if="auth.disablePublicPages" as-child variant="outline" size="sm" class="ml-auto">
          <router-link to="/docs">
            <BookOpen class="h-4 w-4" />
            API Docs
          </router-link>
        </Button>
      </header>
      <main class="flex-1 p-6">
        <RouterView />
      </main>
    </SidebarInset>
  </SidebarProvider>
</template>
