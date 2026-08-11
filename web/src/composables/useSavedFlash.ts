import { ref } from 'vue'

/**
 * A boolean that flips to true for a fixed duration — used for the brief
 * "saved ✓" / "refreshed ✓" flashes after an action completes.
 */
export function useSavedFlash(durationMs = 1500) {
  const active = ref(false)
  let timeout: ReturnType<typeof setTimeout> | null = null

  function flash() {
    active.value = true
    if (timeout) clearTimeout(timeout)
    timeout = setTimeout(() => (active.value = false), durationMs)
  }

  return { active, flash }
}
