import { sveltekit } from '@sveltejs/kit/vite';
import { vite as vidstack } from 'vidstack/plugins';
import { defineConfig } from 'vite';
import Icons from 'unplugin-icons/vite';

const port = process.env.SERVER_PORT || 3000;

export default defineConfig({
	plugins: [sveltekit(), vidstack(), Icons({ compiler: 'svelte' })],
	resolve: {
		alias: {
			'@xivi': './src'
		}
	},
	server: {
		proxy: {
			'/api': {
				target: `http://127.0.0.1:${port}`,
				changeOrigin: true,
				secure: false,
				ws: true
			},
			'/images': {
				target: `http://127.0.0.1:${port}`,
				changeOrigin: true,
				secure: false,
				ws: true
			}
		}
	}
});
