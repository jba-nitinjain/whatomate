/// <reference types="vite/client" />

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<{}, {}, any>
  export default component
}

declare module 'vue3-emoji-picker/css'

interface ImportMetaEnv {
  readonly VITE_API_URL: string
  readonly VITE_WS_URL: string
  readonly VITE_ROLLBAR_ACCESS_TOKEN?: string
  readonly VITE_ROLLBAR_ENVIRONMENT?: string
  readonly VITE_ROLLBAR_CODE_VERSION?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}

interface Window {
  __BASE_PATH__?: string
  __ROLLBAR__?: {
    enabled?: boolean
    access_token?: string
    environment?: string
    code_version?: string
  }
}
