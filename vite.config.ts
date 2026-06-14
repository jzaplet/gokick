import { fileURLToPath, URL } from 'node:url';

import { defineConfig, loadEnv } from 'vite';
import vue from '@vitejs/plugin-vue';
import tailwindcss from '@tailwindcss/vite';
import { sentryVitePlugin } from '@sentry/vite-plugin';

export default defineConfig(({ mode }) => {
    const env = loadEnv(mode, process.cwd(), 'APP_');
    const backendPort = env['APP_HTTP_PORT'] ?? '3000';

    // Sentry source-map upload — only when token + org + project are all present
    // (the release build passes them). Read from process.env, not loadEnv's APP_
    // set: these are build-time CI/Docker secrets, not app .env values, and the
    // token must never reach the bundle.
    const sentryAuthToken = process.env['SENTRY_AUTH_TOKEN'] ?? '';
    const sentryOrg = process.env['SENTRY_ORG'] ?? '';
    const sentryProject = process.env['SENTRY_PROJECT'] ?? '';
    const uploadSourcemaps = sentryAuthToken !== '' && sentryOrg !== '' && sentryProject !== '';

    return {
        plugins: [
            vue(),
            tailwindcss(),
            // Generates + uploads source maps so Sentry de-minifies frontend
            // stack traces, then deletes them so they never reach the embedded
            // public.FS (the Go binary serves it — a stray .map would leak
            // source). Debug IDs (the plugin default) match maps to bundles, so
            // there is no release-name coupling. Omitted entirely without a token.
            ...(uploadSourcemaps
                ? [
                    sentryVitePlugin({
                        org: sentryOrg,
                        project: sentryProject,
                        authToken: sentryAuthToken,
                        telemetry: false,
                        sourcemaps: {
                            filesToDeleteAfterUpload: ['./public/**/*.map'],
                        },
                    }),
                ]
                : []),
        ],
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
            // Maps only when uploading (hidden = no sourceMappingURL comment, so
            // nothing references a .map we then delete); off otherwise so a
            // no-token build embeds zero maps into the binary.
            sourcemap: uploadSourcemaps ? 'hidden' : false,
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
