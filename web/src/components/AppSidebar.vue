<script setup lang="ts">
import { ref, watch } from 'vue'
import { version } from '../../package.json'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { LayoutDashboard, Image, Stamp, Wallpaper, Clapperboard, Settings, LogOut, ChevronRight } from 'lucide-vue-next'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
  SidebarRail,
  useSidebar,
} from '@/components/ui/sidebar'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const { state, isMobile, setOpenMobile } = useSidebar()

const items = [
  { title: 'Dashboard', icon: LayoutDashboard, to: '/admin' },
  { title: 'Posters', icon: Image, to: '/admin/posters' },
  { title: 'Logos', icon: Stamp, to: '/admin/logos' },
  { title: 'Backdrops', icon: Wallpaper, to: '/admin/backdrops' },
  { title: 'Episodes', icon: Clapperboard, to: '/admin/episodes' },
]

const settingsItems = [
  { title: 'General', to: '/admin/settings/general' },
  { title: 'Image', to: '/admin/settings/image' },
]

const isSettingsPath = () => route.path.startsWith('/admin/settings')

// Keep the collapsible in sync with the route: expanded on any settings page,
// collapsed elsewhere (unless the user manually toggles it while already there).
const settingsOpen = ref(isSettingsPath())

watch(
  () => route.path,
  (path) => {
    settingsOpen.value = path.startsWith('/admin/settings')
  },
)

function isActive(path: string) {
  return route.path === path
}

function onNavigate() {
  if (isMobile.value) setOpenMobile(false)
}

function handleLogout() {
  if (isMobile.value) setOpenMobile(false)
  auth.logout()
  router.push('/login')
}
</script>

<template>
  <Sidebar variant="inset" collapsible="icon">
    <SidebarHeader>
      <router-link to="/admin" class="flex flex-col items-center py-1 group-data-[state=expanded]/sidebar-wrapper:items-start hover:opacity-80 transition-opacity" @click="onNavigate">
        <span class="font-bold text-lg whitespace-nowrap">{{ state === 'collapsed' ? 'OPDB' : 'OpenPosterDB' }}</span>
        <span class="text-xs text-muted-foreground whitespace-nowrap">v{{ version }}</span>
      </router-link>
    </SidebarHeader>
    <SidebarContent>
      <SidebarGroup>
        <SidebarMenu>
          <SidebarMenuItem v-for="item in items" :key="item.title">
            <SidebarMenuButton
              as-child
              :is-active="isActive(item.to)"
              :tooltip="item.title"
            >
              <router-link :to="item.to" @click="onNavigate">
                <component :is="item.icon" />
                <span>{{ item.title }}</span>
              </router-link>
            </SidebarMenuButton>
          </SidebarMenuItem>

          <Collapsible v-model:open="settingsOpen" as-child class="group/collapsible">
            <SidebarMenuItem>
              <CollapsibleTrigger as-child>
                <SidebarMenuButton :is-active="isSettingsPath()" tooltip="Settings">
                  <Settings />
                  <span>Settings</span>
                  <ChevronRight class="ml-auto transition-transform duration-200 group-data-[state=open]/collapsible:rotate-90" />
                </SidebarMenuButton>
              </CollapsibleTrigger>
              <CollapsibleContent>
                <SidebarMenuSub>
                  <SidebarMenuSubItem v-for="sub in settingsItems" :key="sub.title">
                    <SidebarMenuSubButton as-child :is-active="isActive(sub.to)">
                      <router-link :to="sub.to" @click="onNavigate">
                        <span>{{ sub.title }}</span>
                      </router-link>
                    </SidebarMenuSubButton>
                  </SidebarMenuSubItem>
                </SidebarMenuSub>
              </CollapsibleContent>
            </SidebarMenuItem>
          </Collapsible>
        </SidebarMenu>
      </SidebarGroup>
    </SidebarContent>
    <SidebarFooter>
      <SidebarMenu>
        <SidebarMenuItem>
          <SidebarMenuButton tooltip="Sign out" @click="handleLogout">
            <LogOut />
            <span>Sign out</span>
          </SidebarMenuButton>
        </SidebarMenuItem>
      </SidebarMenu>
    </SidebarFooter>
    <SidebarRail />
  </Sidebar>
</template>
