import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import { fileURLToPath, URL } from 'node:url';

export default defineConfig(({ mode: _mode }) => {
  // Get build type from environment variable (default: cloud)
  const buildType = process.env.VITE_BUILD_TYPE || 'selfhosted';

  return {
    plugins: [vue()],
    define: {
      // Inject build type as a constant available in the app
      'import.meta.env.VITE_BUILD_TYPE': JSON.stringify(buildType),
    },
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url)),
      },
    },
    server: {
      port: 8080,
      host: true, // Listen on all interfaces (accessible from host)
      proxy: {
        '/api': {
          target: 'http://localhost:5173',
          changeOrigin: true,
          // Forward the original host to the backend
          configure: (proxy, _options) => {
            proxy.on('proxyReq', (proxyReq, req) => {
              // Forward the original host header
              if (req.headers.host) {
                proxyReq.setHeader('X-Forwarded-Host', req.headers.host);
              }
            });
          },
        },
        '/robots.txt': {
          target: 'http://localhost:5173',
          changeOrigin: true,
        },
        '/sitemap.xml': {
          target: 'http://localhost:5173',
          changeOrigin: true,
        },
      },
    },
    build: {
      outDir: 'dist',
      assetsDir: 'assets',
      // 'hidden' emits the .map files but omits the //# sourceMappingURL comment, so
      // browsers do not fetch them and the sources are not served to users, while a
      // stack trace from production can still be symbolicated from the build output.
      // `false` meant every production trace was an unreadable list of minified names.
      sourcemap: 'hidden',
      rollupOptions: {
        output: {
          /*
           * Vendor chunking.
           *
           * The previous `manualChunks` had two problems. The first was ordering:
           * `node_modules/vue` is a *substring* of `node_modules/vue-router` and
           * `node_modules/vue-i18n`, and it was tested first, so those two branches
           * plus `vendor-pinia` and the two date branches below them were unreachable.
           * The second was that it declared chunks Rolldown then merged: any group
           * that is always loaded alongside another gets folded into it, and the
           * surviving chunk keeps *its* name — which is how `vendor-pinia` ended up
           * containing `@vue/shared`, and `vendor-vue` was never emitted at all.
           *
           * So the groups below only draw lines that actually survive a build, and
           * each one is named for exactly what it ends up holding (verified against
           * `dist/`, see the chunk list in the build output):
           *
           *   vendor-framework      vue, vue-router, vue-i18n, pinia and their scoped
           *                         packages. All four load on every page, so Rolldown
           *                         would merge them whatever we called them; one
           *                         honestly named chunk beats four fictional ones.
           *   vendor-date-holidays  the 1.4 MB holiday dataset and its dependency
           *                         tree. Genuinely separate, because
           *                         utils/calendar/holidays.ts imports it dynamically.
           *   vendor-date           the timezone tables.
           *
           * axios and @vueuse are deliberately absent: axios is always loaded and
           * folds into the entry, and @vueuse is only reachable from ParticipantView,
           * so it already travels with it. Declaring groups for them would just
           * recreate the dead branches this replaces.
           */
          codeSplitting: {
            groups: [
              {
                name: 'vendor-date-holidays',
                test: /node_modules[\\/](date-holidays|date-holidays-parser|astronomia|caldate|date-bengali-revised|date-chinese|date-easter|jalaali-js|moment|moment-timezone)[\\/]/,
              },
              {
                name: 'vendor-framework',
                test: /node_modules[\\/](vue|@vue|vue-router|vue-i18n|@intlify|pinia)[\\/]/,
              },
              {
                name: 'vendor-date',
                test: /node_modules[\\/](date-fns|date-fns-tz|countries-and-timezones)[\\/]/,
              },
            ],
          },
        },
      },
      // Increase chunk size warning limit for known large libraries
      chunkSizeWarningLimit: 600,
    },
  };
});
