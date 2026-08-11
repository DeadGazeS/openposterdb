<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import { version } from '../../package.json'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { adminApi } from '@/lib/api'
import { LayoutDashboard, Image, Stamp, Wallpaper, Clapperboard, Settings, LogOut, ChevronRight, Github, ExternalLink, Minus, Plus } from 'lucide-vue-next'
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
  { title: 'API', to: '/admin/settings/api' },
  { title: 'Global Image Settings', to: '/admin/settings/image' },
  { title: 'Backup', to: '/admin/settings/backup' },
]

// New-version indicator: the highest semver container tag published to the
// user's fork beats the local package version → show a small link. Fails
// silently (no indicator) on any network/API error.
const latestVersion = ref<string | null>(null)
const latestVersionUrl = 'https://github.com/DeadGazeS/openposterdb/pkgs/container/openposterdb'

// Disclaimer minimisation: clicking the header row (between "Disclaimer:" and
// the dash, or the dash itself) collapses the box to just the title. The
// collapsed state is stored per admin user on the server (survives login
// sessions); localStorage is only a cache for the first paint before prefs load.
const DISCLAIMER_KEY = 'sidebar-disclaimer-minimised'
const disclaimerOpen = ref(localStorage.getItem(DISCLAIMER_KEY) !== 'minimised')

onMounted(async () => {
  try {
    const res = await adminApi.getPrefs()
    if (res.ok) {
      const prefs = (await res.json()) as Record<string, string>
      disclaimerOpen.value = prefs.disclaimer_minimised !== 'minimised'
    }
  } catch {
    // prefs unavailable: keep the local default
  }
})

watch(disclaimerOpen, (open) => {
  localStorage.setItem(DISCLAIMER_KEY, open ? 'open' : 'minimised')
  void adminApi.updatePrefs({ disclaimer_minimised: open ? 'open' : 'minimised' })
})

function parseVersion(v: string): number[] {
  const m = v.replace(/^v/, '').match(/^(\d+)\.(\d+)\.(\d+)/)
  return m ? [Number(m[1]), Number(m[2]), Number(m[3])] : []
}

function isNewer(a: number[], b: number[]): boolean {
  for (let i = 0; i < 3; i++) {
    if (a[i]! > b[i]!) return true
    if (a[i]! < b[i]!) return false
  }
  return false
}

onMounted(async () => {
  try {
    const res = await fetch(
      'https://api.github.com/repos/DeadGazeS/openposterdb/releases/latest',
    )
    if (!res.ok) return
    const release = (await res.json()) as { tag_name?: string }
    const tag = release.tag_name
    if (!tag) return
    const remote = parseVersion(tag)
    const local = parseVersion(version)
    if (remote.length === 3 && local.length === 3 && isNewer(remote, local)) {
      latestVersion.value = tag
    }
  } catch {
    // network/API errors: no indicator
  }
})

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
        <span class="flex items-center gap-1.5 text-xs text-muted-foreground whitespace-nowrap">
          v{{ version }}
          <a
            v-if="latestVersion"
            :href="latestVersionUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="inline-flex items-center gap-0.5 rounded-full bg-amber-500/15 px-1.5 py-px font-semibold text-amber-600 dark:text-amber-400 hover:underline"
            :title="`New version ${latestVersion} available`"
            @click.stop
          >
            <ExternalLink class="size-3" />
            {{ latestVersion }}
          </a>
        </span>
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
          <div
            v-if="state !== 'collapsed'"
            class="space-y-1 rounded-md border border-amber-500/30 bg-amber-500/10 px-2 py-1.5 text-xs text-muted-foreground"
          >
            <div
              class="flex items-center justify-between gap-2 cursor-pointer select-none"
              @click="disclaimerOpen = !disclaimerOpen"
            >
              <p class="font-semibold text-amber-600 dark:text-amber-400">Disclaimer:</p>
              <span
                class="inline-flex size-4 shrink-0 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-amber-500/20 hover:text-amber-600 dark:hover:text-amber-400"
                :title="disclaimerOpen ? 'Minimise' : 'Expand'"
                aria-hidden="true"
              >
                <Minus v-if="disclaimerOpen" class="size-3" />
                <Plus v-else class="size-3" />
              </span>
            </div>
            <template v-if="disclaimerOpen">
              <p>
                Most changes I made were AI generated. I am a developer but didn't want to read myself into
                Rust (original language of this project, developed by
                <a href="https://github.com/PNRxA" target="_blank" rel="noopener noreferrer" class="underline underline-offset-2 hover:text-foreground">PNRxA</a>)
                or Go as of now.
              </p>
              <p>
                <a href="https://github.com/PNRxA/openposterdb" target="_blank" rel="noopener noreferrer" class="underline underline-offset-2 hover:text-foreground">Original project</a>
              </p>
            </template>
          </div>
        </SidebarMenuItem>
        <SidebarMenuItem>
          <SidebarMenuButton as-child tooltip="GitHub repository">
            <a href="https://github.com/DeadGazeS/openposterdb" target="_blank" rel="noopener noreferrer">
              <Github />
              <span>GitHub</span>
            </a>
          </SidebarMenuButton>
        </SidebarMenuItem>
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
