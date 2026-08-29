import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import { createHash } from 'node:crypto';
import * as mediaChrome from 'media-chrome';

const mobileBuild = process.argv.includes('mobile') || process.env.XIVI_MOBILE === '1';

// Media Chrome creates its web-component styles inside shadow roots at runtime,
// outside SvelteKit's normal CSP hash collection. Generate exact hashes from the
// installed package instead of broadly enabling arbitrary inline styles. A
// dependency update therefore refreshes the allowlist during the next build.
const mediaChromeStyleHashes = new Set();
for (const component of Object.values(mediaChrome)) {
	if (typeof component?.getTemplateHTML !== 'function') continue;
	let template = '';
	try {
		template = component.getTemplateHTML({});
	} catch {
		continue;
	}
	for (const match of template.matchAll(/<style>([\s\S]*?)<\/style>/g)) {
		mediaChromeStyleHashes.add(`sha256-${createHash('sha256').update(match[1]).digest('base64')}`);
	}
}

/** @type {import('@sveltejs/kit').Config} */
const config = {
	// Consult https://kit.svelte.dev/docs/integrations#preprocessors
	// for more information about preprocessors
	preprocess: vitePreprocess(),

	kit: {
		adapter: adapter({
			pages: 'build',
			assets: 'build',
			fallback: 'index.html',
			precompress: !mobileBuild,
			strict: true
		}),
		alias: {
			'@xivi': './src',
			$lib: './src/lib'
		},
		csp: {
			mode: 'hash',
			directives: {
				'default-src': ['self'],
				'base-uri': ['none'],
				'object-src': ['none'],
				'form-action': ['self'],
				'script-src': ['self'],
				'style-src': ['self', ...mediaChromeStyleHashes],
				'style-src-attr': ['unsafe-inline'],
				'font-src': ['self'],
				'img-src': ['self', 'data:', 'blob:'],
				'media-src': ['self', 'blob:'],
				'connect-src': ['self'],
				'worker-src': ['self', 'blob:']
			}
		}
	}
};

export default config;
