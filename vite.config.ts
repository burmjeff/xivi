import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';
import tailwindcss from '@tailwindcss/vite';

const port = process.env.SERVER_PORT || 3000;

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
