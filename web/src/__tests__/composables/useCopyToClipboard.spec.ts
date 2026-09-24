import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { useCopyToClipboard } from '@/composables/useCopyToClipboard'

describe('useCopyToClipboard', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('writes to navigator.clipboard and flashes "copied" for the given key', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, { clipboard: { writeText } })

    const { copyState, copy } = useCopyToClipboard()
    expect(copyState.value['a']).toBeUndefined()

    await copy('hello', 'a')

    expect(writeText).toHaveBeenCalledWith('hello')
    expect(copyState.value['a']).toBe('copied')

    vi.advanceTimersByTime(1500)
    expect(copyState.value['a']).toBe('idle')
  })

  it('keys are independent — copying one does not flip another', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, { clipboard: { writeText } })

    const { copyState, copy } = useCopyToClipboard()
    await copy('one', 'a')
    await copy('two', 'b')

    expect(copyState.value['a']).toBe('copied')
    expect(copyState.value['b']).toBe('copied')
  })

  it('falls back to document.execCommand when navigator.clipboard.writeText rejects', async () => {
    Object.assign(navigator, {
      clipboard: { writeText: vi.fn().mockRejectedValue(new Error('denied')) },
    })
    const execCommand = vi.fn().mockReturnValue(true)
    document.execCommand = execCommand

    const { copyState, copy } = useCopyToClipboard()
    await copy('fallback text', 'x')

    expect(execCommand).toHaveBeenCalledWith('copy')
    expect(copyState.value['x']).toBe('copied')
  })

  it('falls back to document.execCommand when navigator.clipboard is unavailable', async () => {
    Object.assign(navigator, { clipboard: undefined })
    const execCommand = vi.fn().mockReturnValue(true)
    document.execCommand = execCommand

    const { copyState, copy } = useCopyToClipboard()
    await copy('no clipboard api', 'y')

    expect(execCommand).toHaveBeenCalledWith('copy')
    expect(copyState.value['y']).toBe('copied')
  })

  it('respects a custom flash duration', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, { clipboard: { writeText } })

    const { copyState, copy } = useCopyToClipboard(500)
    await copy('quick', 'z')

    expect(copyState.value['z']).toBe('copied')
    vi.advanceTimersByTime(499)
    expect(copyState.value['z']).toBe('copied')
    vi.advanceTimersByTime(1)
    expect(copyState.value['z']).toBe('idle')
  })
})
