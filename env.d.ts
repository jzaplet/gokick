/// <reference types="vite/client" />

interface ImportMeta {
    readonly dirname: string;
}

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<{}, {}, any>
  export default component
}
