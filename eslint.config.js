import js from '@eslint/js';

export default [
	js.configs.recommended,
	{
		ignores: ['*.cjs', 'build/', '.svelte-kit/', 'node_modules/', 'dist/', 'coverage/']
	},
	{
		files: ['**/*.{js,ts}'],
		languageOptions: {
			ecmaVersion: 2020,
			sourceType: 'module',
			globals: {
				console: 'readonly',
				process: 'readonly',
				Buffer: 'readonly',
				__dirname: 'readonly',
				__filename: 'readonly',
				exports: 'writable',
				global: 'readonly',
				module: 'readonly',
				require: 'readonly'
			}
		}
	}
];
