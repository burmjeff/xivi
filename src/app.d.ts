// See https://kit.svelte.dev/docs/types#app
// for information about these interfaces
// and what to do when importing types
/// <reference types="unplugin-icons/types/svelte" />
/// <reference types="vite/client" />

declare namespace App {
	// interface Locals {}
	// interface PageData {}
	// interface Error {}
	// interface Platform {}
}

// Extend ImportMeta interface to include Vite HMR API
interface ImportMeta {
	readonly hot?: {
		readonly data: any;
		accept(): void;
		accept(cb: (mod: any) => void): void;
		accept(dep: string, cb: (mod: any) => void): void;
		accept(deps: readonly string[], cb: (mods: any[]) => void): void;
		dispose(cb: (data: any) => void): void;
		decline(): void;
		invalidate(): void;
		on(event: string, cb: (data: any) => void): void;
		send(event: string, data?: any): void;
	};
}
