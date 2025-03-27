import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';
import { vite as vidstack } from 'vidstack/plugins';
import Icons from 'unplugin-icons/vite';
import tailwindcss from '@tailwindcss/vite';

const port = process.env.SERVER_PORT || 3000;

export default defineConfig({
  plugins: [
	tailwindcss(),
    sveltekit(),
    vidstack(),
    Icons({ compiler: 'svelte' })
  ],
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
