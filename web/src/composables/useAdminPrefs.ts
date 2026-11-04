// useAdminPrefs — the per-admin UI preferences (GET /api/admin/prefs) as one
// shared query, so the sidebar and the dashboard don't each fetch them, and
// API-key sessions (which can't read them) never ask.

import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { adminApi } from '@/lib/api'

export type AdminPrefs = Record<string, string>

export const ADMIN_PREFS_QUERY_KEY = ['admin-prefs'] as const

export function useAdminPrefs(enabled: MaybeRefOrGetter<boolean> = true) {
  const queryClient = useQueryClient()
  const query = useQuery<AdminPrefs>({
    queryKey: ADMIN_PREFS_QUERY_KEY,
    queryFn: async () => {
      const res = await adminApi.getPrefs()
      if (!res.ok) throw new Error(`prefs: HTTP ${res.status}`)
      return (await res.json()) as AdminPrefs
    },
    enabled: computed(() => toValue(enabled)),
    retry: false,
    staleTime: 60_000,
  })

  /** Saves some prefs and keeps the shared copy in step. Returns ok. */
  async function savePrefs(patch: AdminPrefs): Promise<boolean> {
    const res = await adminApi.updatePrefs(patch)
    if (res.ok) {
      queryClient.setQueryData<AdminPrefs>(ADMIN_PREFS_QUERY_KEY, (old) => ({ ...(old ?? {}), ...patch }))
    }
    return res.ok
  }

  return { prefs: query.data, isPending: query.isPending, isError: query.isError, savePrefs }
}
