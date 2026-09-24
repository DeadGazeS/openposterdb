import { ref } from 'vue'

/**
 * Copy-to-clipboard with a legacy execCommand fallback (older browsers /
 * non-secure contexts) and a brief per-key 'copied' flash for UI feedback.
 * Keyed so a single instance can back many copy buttons at once (e.g. one
 * per table row).
 */
export function useCopyToClipboard(flashMs = 1500) {
  const copyState = ref<Record<string, 'idle' | 'copied'>>({})

  function flash(key: string) {
    copyState.value[key] = 'copied'
    setTimeout(() => {
      copyState.value = { ...copyState.value, [key]: 'idle' }
    }, flashMs)
  }

  async function copy(text: string, key: string) {
    try {
      await navigator.clipboard.writeText(text)
      flash(key)
    } catch {
      // Older browsers / non-secure contexts — fall back to the legacy
      // document.execCommand path so the copy still works.
      const el = document.createElement('textarea')
      el.value = text
      el.style.position = 'fixed'
      el.style.opacity = '0'
      document.body.appendChild(el)
      el.select()
      try {
        document.execCommand('copy')
        flash(key)
      } finally {
        document.body.removeChild(el)
      }
    }
  }

  return { copyState, copy }
}
