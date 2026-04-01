import { onBeforeUnmount, onMounted, ref } from 'vue'

interface UseViewRefreshOptions {
  intervalMs?: number
  minGapMs?: number
  refreshOnFocus?: boolean
  refreshOnVisible?: boolean
}

export function useViewRefresh(
  refreshFn: () => Promise<unknown> | unknown,
  {
    intervalMs = 0,
    minGapMs = 15000,
    refreshOnFocus = true,
    refreshOnVisible = true
  }: UseViewRefreshOptions = {}
) {
  const isRefreshing = ref(false)
  let lastRefreshAt = 0
  let intervalId: number | null = null

  async function refreshNow(force = false) {
    const now = Date.now()
    if (isRefreshing.value) return false
    if (!force && lastRefreshAt && now - lastRefreshAt < minGapMs) return false

    isRefreshing.value = true
    try {
      await refreshFn()
      lastRefreshAt = Date.now()
      return true
    } finally {
      isRefreshing.value = false
    }
  }

  const handleVisible = () => {
    if (document.visibilityState === 'visible') {
      void refreshNow()
    }
  }

  const handleFocus = () => {
    if (document.visibilityState === 'visible') {
      void refreshNow()
    }
  }

  onMounted(() => {
    if (refreshOnVisible) {
      document.addEventListener('visibilitychange', handleVisible)
    }

    if (refreshOnFocus) {
      window.addEventListener('focus', handleFocus)
    }

    if (intervalMs > 0) {
      intervalId = window.setInterval(() => {
        if (document.visibilityState === 'visible') {
          void refreshNow()
        }
      }, intervalMs)
    }
  })

  onBeforeUnmount(() => {
    if (refreshOnVisible) {
      document.removeEventListener('visibilitychange', handleVisible)
    }

    if (refreshOnFocus) {
      window.removeEventListener('focus', handleFocus)
    }

    if (intervalId !== null) {
      window.clearInterval(intervalId)
      intervalId = null
    }
  })

  return {
    isRefreshing,
    refreshNow
  }
}
