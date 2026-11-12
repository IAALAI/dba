import { createApp } from 'vue'
import router from './router'
import App from './Body.vue'

createApp(App)
    .use(router)
    .mount('body')
