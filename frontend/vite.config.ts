import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'

export default defineConfig(({ mode: _mode }) => {
  // Get build type from environment variable (default: cloud)
  const buildType = process.env.VITE_BUILD_TYPE || 'selfhosted'

  return {
  plugins: [vue()],
  define: {
    // Inject build type as a constant available in the app
    'import.meta.env.VITE_BUILD_TYPE': JSON.stringify(buildType)
  },
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url))
    }
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
        }
      },
      '/robots.txt': {
        target: 'http://localhost:5173',
        changeOrigin: true,
      },
      '/sitemap.xml': {
        target: 'http://localhost:5173',
        changeOrigin: true,
      }
    }
  },
  build: {
    outDir: 'dist',
    assetsDir: 'assets',
    sourcemap: false,
    rollupOptions: {
      output: {
        manualChunks: (id) => {
          // Vendor chunks (frameworks)
          if (id.includes('node_modules/vue') || id.includes('node_modules/@vue')) {
            return 'vendor-vue'
          }
          if (id.includes('node_modules/vue-router')) {
            return 'vendor-router'
          }
          if (id.includes('node_modules/pinia')) {
            return 'vendor-pinia'
          }

          // Date utilities - split date-holidays separately (large library)
          if (id.includes('node_modules/date-holidays')) {
            return 'date-holidays' // Lazy loaded chunk
          }
          if (id.includes('node_modules/date-fns')) {
            return 'date-fns'
          }

          // Other utilities
          if (id.includes('node_modules/@vueuse')) {
            return 'vueuse'
          }
          if (id.includes('node_modules/axios')) {
            return 'axios'
          }
          if (id.includes('node_modules/vue-i18n')) {
            return 'i18n'
          }
        }
      }
    },
    // Increase chunk size warning limit for known large libraries
    chunkSizeWarningLimit: 600
  }
  }
})
