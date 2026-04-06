import { fileURLToPath, URL } from 'node:url';

import { defineConfig, loadEnv } from 'vite';
import vue from '@vitejs/plugin-vue';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig(({ mode }) => {
    const env = loadEnv(mode, process.cwd(), 'APP_');
    const backendPort = env['APP_HTTP_PORT'] ?? '3000';

    return {
        plugins: [vue(), tailwindcss()],
        resolve: {
            // Maps '@' to './assets/' so imports like '@/vue/App.vue' work at runtime.
            // Mirrors tsconfig.json paths which only covers type-checking.
            alias: {
                '@': fileURLToPath(new URL('./assets/', import.meta.url)),
            },
        },
        build: {
            outDir: 'public',
            // Keep embed.go and favicon.ico — Vite only writes its own output files.
            emptyOutDir: false,
        },
        server: {
            proxy: {
                // Forward API calls and favicon to the Go backend during development.
                // Port is read from APP_HTTP_PORT in .env (default: 3000).
                '^/(api|health|favicon\\.ico)': {
                    target: `http://localhost:${backendPort}`,
                    changeOrigin: true,
                },
            },
        },
        // Disabled because we build directly into public/ which already contains embed.go.
        // With publicDir enabled, Vite would try to copy public/ into itself.
        publicDir: false,
        test: {
            root: '.',
            include: ['tests/**/*.test.ts'],
            environment: 'jsdom',
        },
    };
});
