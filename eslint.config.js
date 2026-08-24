import js from '@eslint/js';
import tsParser from '@typescript-eslint/parser';
import svelte from 'eslint-plugin-svelte';
import globals from 'globals';

export default [
	{
		ignores: [
			'build/',
			'.svelte-kit/',
			'node_modules/',
			'dist/',
			'coverage/',
			'src/lib/api/generated.ts'
		]
	},
	js.configs.recommended,
	...svelte.configs['flat/recommended'],
	{
		files: ['**/*.ts'],
		languageOptions: {
			parser: tsParser,
			parserOptions: { sourceType: 'module' },
			globals: { ...globals.browser, ...globals.node }
		},
		rules: { 'no-unused-vars': 'off', 'no-undef': 'off' }
	},
	{
		files: ['**/*.svelte'],
		languageOptions: {
			parserOptions: { parser: tsParser },
			globals: { ...globals.browser }
		},
		rules: {
			'no-unused-vars': 'off',
			'no-undef': 'off',
			'svelte/no-navigation-without-resolve': 'off',
			'svelte/require-each-key': 'off',
			'svelte/prefer-svelte-reactivity': 'off',
			'svelte/no-unused-svelte-ignore': 'off'
		}
	},
	{
		files: ['**/*.js'],
		languageOptions: {
			ecmaVersion: 2022,
			sourceType: 'module',
			globals: { ...globals.node }
		}
	}
];
