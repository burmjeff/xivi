import { readFileSync, readdirSync, existsSync } from 'node:fs';
import { resolve, join } from 'node:path';

/** @param {string | undefined} [value] */
export function sourceInfo(value) {
	if (!value)
		return {
			sourceUrl: 'https://github.com/burmjeff/xivi',
			sourceLabel: 'Upstream project source'
		};
	const url = new URL(value);
	if (url.protocol !== 'https:' || url.username || url.password) {
		throw new Error('VITE_XIVI_SOURCE_URL must be an HTTPS source URL without credentials');
	}
	return { sourceUrl: url.href, sourceLabel: 'Source for this build' };
}

/** @param {string} root */
export function npmNotices(root) {
	const lock = JSON.parse(readFileSync(join(root, 'package-lock.json'), 'utf8'));
	const notices = [];
	for (const [path, metadata] of Object.entries(lock.packages)) {
		if (!path) continue;
		const directory = resolve(root, path);
		// npm omits optional packages for other platforms. No source code from them
		// is shipped by this build. Include installed build tools conservatively.
		if (!existsSync(directory)) continue;
		const files = readdirSync(directory, { withFileTypes: true })
			.filter(
				(entry) => entry.isFile() && /^(licen[cs]e|copying|notice|ofl)([.-]|$)/i.test(entry.name)
			)
			.map((entry) => entry.name)
			.sort();
		const name = path.replace(/^.*node_modules\//, '');
		const heading = `${name}@${metadata.version} — ${metadata.license || 'See bundled license'}`;
		if (!files.length) {
			// A missing notice is visible in the inventory; never fabricate a grant.
			notices.push(
				`${heading}\nNo root license file in this package. Review upstream notices before redistribution.`
			);
			continue;
		}
		notices.push(
			`${heading}\n${files.map((file) => `${file}\n${readFileSync(join(directory, file), 'utf8')}`).join('\n')}`
		);
	}
	return notices.join('\n\n' + '='.repeat(72) + '\n\n');
}

/**
 * @param {string} root
 * @param {string | undefined} sourceUrl
 * @returns {import('vite').Plugin}
 */
export function legalPlugin(root, sourceUrl) {
	let cached;
	const content = () =>
		(cached ??= {
			...sourceInfo(sourceUrl),
			license: readFileSync(join(root, 'LICENSE'), 'utf8'),
			notice: readFileSync(join(root, 'NOTICE'), 'utf8'),
			thirdParty: readFileSync(join(root, 'THIRD_PARTY_NOTICES.md'), 'utf8'),
			dependencyNotices: npmNotices(root)
		});
	return {
		name: 'xivi-legal-notices',
		resolveId(id) {
			if (id === 'virtual:xivi-legal') return '\0virtual:xivi-legal';
		},
		load(id) {
			if (id === '\0virtual:xivi-legal') return `export default ${JSON.stringify(content())};`;
		},
		generateBundle() {
			const data = content();
			for (const [fileName, source] of Object.entries({
				'LICENSE.txt': data.license,
				'NOTICE.txt': data.notice,
				'THIRD_PARTY_NOTICES.txt': data.thirdParty,
				'WEB_DEPENDENCY_NOTICES.txt': data.dependencyNotices,
				'source.json': JSON.stringify({ sourceUrl: data.sourceUrl, sourceLabel: data.sourceLabel })
			}))
				this.emitFile({ type: 'asset', fileName: `legal/${fileName}`, source });
		}
	};
}
