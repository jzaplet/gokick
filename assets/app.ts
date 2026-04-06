import { createApp } from 'vue';
import App from '@/App.vue';
import { router } from '@/router';
import '@/tailwind.css';
import '@/img/go-vue-cqrs-ddd.png';

createApp(App).use(router).mount('#app');
