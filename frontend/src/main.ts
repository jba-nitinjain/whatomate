import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { VueQueryPlugin } from '@tanstack/vue-query'

import App from './App.vue'
import router from './router'
import { i18n } from './i18n'
import { installRollbar, setRollbarContext } from './services/rollbar'

import './assets/fonts.css'
import './assets/index.css'

const app = createApp(App)

installRollbar(app)

app.use(createPinia())
app.use(router)
app.use(VueQueryPlugin)
app.use(i18n)

router.afterEach((to) => {
  setRollbarContext(typeof to.name === 'string' ? to.name : to.fullPath)
})

app.mount('#app')
