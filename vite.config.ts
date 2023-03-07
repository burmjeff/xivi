import { sveltekit } from '@sveltejs/kit/vite';
import type { UserConfig } from 'vite';
import Icons from 'unplugin-icons/vite';

const port = process.env.SERVER_PORT || 8080;

const config: UserConfig = {
	plugins: [
		sveltekit(),
		Icons({ compiler: 'svelte' })
	],
	resolve: {
		alias: {
			'@xivi': './src'
		}
	},
	server: {
		proxy: {
		  "/api": {
			target: `http://127.0.0.1:${port}`,
			changeOrigin: true,
			secure: false,
			ws: true,
		  },
		},
	},
};

export default config;
