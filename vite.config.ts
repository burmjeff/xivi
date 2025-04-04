import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';
import { vite as vidstack } from 'vidstack/plugins';
import Icons from 'unplugin-icons/vite';
import tailwindcss from '@tailwindcss/vite';

const port = process.env.SERVER_PORT || 3000;

export default defineConfig({
  optimizeDeps: {
    // Pre-bundle these dependencies for faster startup
    include: [
      'svelte', 
      '@sveltejs/kit', 
      'svelte-dnd-action',
      '@skeletonlabs/skeleton-svelte',
      '@floating-ui/dom'
    ],
    // Disable force bundling to use cache when possible
    force: false
  },
  plugins: [
    tailwindcss(),
    sveltekit(),
    vidstack(),
    Icons({ compiler: 'svelte' })
  ],
  resolve: {
    alias: {
      '@xivi': './src'
    },
    // Deduplicate packages that might be causing issues
    dedupe: ['svelte', '@sveltejs/kit']
  },
  server: {
    // Use IPv4 explicitly to avoid IPv6 issues
    host: '0.0.0.0',
    port: 5173,
    strictPort: true,
    hmr: {
      // Use WebSockets for HMR
      protocol: 'ws',
      // Increase timeout for stability
      timeout: 5000,
      // Disable overlay for better performance
      overlay: false
    },
    // Disable file system polling for better performance
    watch: {
      usePolling: false
    },
    // Optimize proxy settings
    proxy: {
      '/api': {
        target: `http://127.0.0.1:${port}`,
        changeOrigin: true,
        secure: false,
        ws: true,
        // Add timeout to prevent hanging requests
        timeout: 5000
      },
      '/images': {
        target: `http://127.0.0.1:${port}`,
        changeOrigin: true,
        secure: false,
        ws: true,
        timeout: 5000
      },
      '/proxy-image': {
        target: `http://127.0.0.1:${port}`,
        changeOrigin: true,
        secure: false,
        ws: true,
        timeout: 5000
      }
    }
  },
  css: {
    // Force consistent CSS ordering in development
    devSourcemap: true,
  },
  // Optimize build settings
  build: {
    // Target modern browsers for better performance
    target: 'esnext',
    // Disable compressed size reporting to speed up build
    reportCompressedSize: false,
    // Increase chunk size warning limit
    chunkSizeWarningLimit: 1000,
    // Optimize CSS handling
    cssCodeSplit: true,
    // Improve minification
    minify: 'esbuild'
  },
  // Improve dependency optimization
  esbuild: {
    logOverride: { 'this-is-undefined-in-esm': 'silent' }
  },
  // Improve caching
  cacheDir: '.vite'
});