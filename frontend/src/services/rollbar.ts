import type { App, ComponentPublicInstance } from 'vue'
import type { AxiosError } from 'axios'
import Rollbar from 'rollbar'

type RollbarUser = {
  id: string
  email?: string
  full_name?: string
}

type RuntimeRollbarConfig = {
  enabled?: boolean
  access_token?: string
  environment?: string
  code_version?: string
}

const runtimeConfig = (window as Window).__ROLLBAR__ as RuntimeRollbarConfig | undefined
const accessToken =
  runtimeConfig?.access_token?.trim() ||
  import.meta.env.VITE_ROLLBAR_ACCESS_TOKEN?.trim() ||
  ''
const environment =
  runtimeConfig?.environment?.trim() ||
  import.meta.env.VITE_ROLLBAR_ENVIRONMENT?.trim() ||
  'development'
const codeVersion =
  runtimeConfig?.code_version?.trim() ||
  import.meta.env.VITE_ROLLBAR_CODE_VERSION?.trim() ||
  undefined
const rollbarEnabled = accessToken !== '' && runtimeConfig?.enabled !== false

const rollbar = rollbarEnabled
  ? new Rollbar({
      accessToken,
      captureUncaught: true,
      captureUnhandledRejections: true,
      payload: {
        environment,
        context: window.location.pathname,
        client: {
          javascript: {
            code_version: codeVersion,
          },
        },
      },
    })
  : null

syncRollbarPersonFromStorage()

export function installRollbar(app: App): void {
  if (!rollbar) {
    return
  }

  const existingErrorHandler = app.config.errorHandler

  app.config.errorHandler = (error, instance, info) => {
    reportError(error, {
      source: 'vue',
      info,
      component: getComponentName(instance),
    })
    existingErrorHandler?.(error, instance, info)
  }

  app.provide('rollbar', rollbar)
}

export function setRollbarContext(context: string): void {
  if (!rollbar) {
    return
  }
  rollbar.configure({
    payload: {
      context,
    },
  })
}

export function setRollbarPerson(user: RollbarUser | null): void {
  if (!rollbar) {
    return
  }

  rollbar.configure({
    payload: {
      person: user
        ? {
            id: user.id,
            email: user.email,
            username: user.full_name,
          }
        : {
            id: null,
          },
    },
  })
}

export function clearRollbarPerson(): void {
  setRollbarPerson(null)
}

export function reportError(error: unknown, extra: Record<string, unknown> = {}): void {
  if (!rollbar) {
    return
  }

  if (error instanceof Error) {
    rollbar.error(error, extra)
    return
  }

  const message = normalizeMessage(error)
  if (message !== '') {
    rollbar.error(message, extra)
  }
}

export function reportApiError(
  error: unknown,
  extra: Record<string, unknown> = {},
): void {
  if (!rollbar) {
    return
  }

  if (isAxiosError(error)) {
    const method = String(error.config?.method || 'GET').toUpperCase()
    const url = String(error.config?.url || '')
    const status = error.response?.status
    const responseMessage = getAxiosResponseMessage(error)
    const details: Record<string, unknown> = {
      source: 'axios',
      method,
      url,
      status_code: status,
      response_message: responseMessage || undefined,
      ...extra,
    }

    if (status && status >= 400 && status < 500) {
      const message = responseMessage
        ? `API ${method} ${url} failed with ${status} - ${responseMessage}`
        : `API ${method} ${url} failed with ${status}`
      rollbar.warning(message, details)
      return
    }

    if (error instanceof Error) {
      rollbar.error(error, details)
      return
    }

    rollbar.error(`API ${method} ${url} failed`, details)
    return
  }

  reportError(error, {
    source: 'api',
    ...extra,
  })
}

function syncRollbarPersonFromStorage(): void {
  try {
    const storedUser = localStorage.getItem('user')
    if (!storedUser) {
      clearRollbarPerson()
      return
    }

    const parsed = JSON.parse(storedUser)
    if (!parsed || typeof parsed !== 'object' || !parsed.id) {
      clearRollbarPerson()
      return
    }

    setRollbarPerson({
      id: String(parsed.id),
      email: typeof parsed.email === 'string' ? parsed.email : undefined,
      full_name: typeof parsed.full_name === 'string' ? parsed.full_name : undefined,
    })
  } catch {
    clearRollbarPerson()
  }
}

function getComponentName(instance: ComponentPublicInstance | null): string | undefined {
  if (!instance) {
    return undefined
  }

  const namedType = instance.$?.type as { name?: string } | undefined
  if (namedType?.name) {
    return namedType.name
  }

  const proxyType = instance.$options as { name?: string } | undefined
  return proxyType?.name
}

function normalizeMessage(error: unknown): string {
  if (typeof error === 'string') {
    return error
  }
  if (error instanceof Error) {
    return error.message
  }
  if (error === null || error === undefined) {
    return ''
  }
  return String(error)
}

function isAxiosError(
  error: unknown,
): error is AxiosError<{ message?: string; errors?: Array<{ message?: string } | string> }> {
  return (
    typeof error === 'object' &&
    error !== null &&
    'isAxiosError' in error &&
    (error as { isAxiosError?: boolean }).isAxiosError === true
  )
}

function getAxiosResponseMessage(
  error: AxiosError<{ message?: string; errors?: Array<{ message?: string } | string> }>,
): string {
  const responseMessage = error.response?.data?.message
  if (typeof responseMessage === 'string' && responseMessage.trim() !== '') {
    return responseMessage.trim()
  }

  const firstError = error.response?.data?.errors?.[0]
  if (typeof firstError === 'string' && firstError.trim() !== '') {
    return firstError.trim()
  }
  if (
    firstError &&
    typeof firstError === 'object' &&
    'message' in firstError &&
    typeof firstError.message === 'string' &&
    firstError.message.trim() !== ''
  ) {
    return firstError.message.trim()
  }

  return error.message?.trim() || ''
}
