import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';
import tailwindcss from '@tailwindcss/vite';

const port = process.env.SERVER_PORT || 3000;
const backend = `http://127.0.0.1:${port}`;

export default defineConfig({
	plugins: [tailwindcss(), sveltekit()],
	server: {
		host: '0.0.0.0',
		port: 5173,
		strictPort: true,
		hmr: {
			overlay: true
		},
		proxy: {
			'/docs': {
				target: backend,
				changeOrigin: false,
				secure: false,
				timeout: 30_000
			},
			'/swagger': {
				target: backend,
				changeOrigin: false,
				secure: false,
				timeout: 30_000
			},
			'/media': {
				target: backend,
				changeOrigin: true,
				secure: false,
				timeout: 0,
				proxyTimeout: 0
			},
			'/stream': {
				target: backend,
				changeOrigin: true,
				secure: false,
				timeout: 0,
				proxyTimeout: 0
			},
			'/api': {
				target: backend,
				// CSRF validation compares the browser Origin with the effective
				// request host. Preserve the browser-facing Vite host instead of
				// replacing it with the backend's private development address.
				changeOrigin: false,
				secure: false,
				ws: true,
				// Add timeout to prevent hanging requests
				timeout: 5000
			},
			'/images': {
				target: backend,
				changeOrigin: true,
				secure: false,
				ws: true,
				timeout: 5000
			},
			'/virtual-tuner': {
				target: backend,
				changeOrigin: true,
				secure: false,
				timeout: 0,
				proxyTimeout: 0
			}
		}
	},
	build: {
		target: 'esnext',
		reportCompressedSize: false,
		chunkSizeWarningLimit: 1000,
		cssCodeSplit: true,
		minify: 'esbuild'
	},
	esbuild: {
		logOverride: { 'this-is-undefined-in-esm': 'silent' }
	},
	cacheDir: '.vite'
});
