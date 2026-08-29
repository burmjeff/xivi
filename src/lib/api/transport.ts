import { isNativePlatform, XiviNative } from '$lib/platform/native';

export interface ApiTransport {
	request(path: string, init?: RequestInit): Promise<Response>;
}

class BrowserApiTransport implements ApiTransport {
	request(path: string, init?: RequestInit) {
		return fetch(path, { ...init, credentials: 'same-origin', cache: 'no-store' });
	}
}

class NativeApiTransport implements ApiTransport {
	async request(path: string, init?: RequestInit) {
		const headers: Record<string, string> = {};
		new Headers(init?.headers).forEach((value, key) => (headers[key] = value));
		const result = await XiviNative.request({
			path,
			method: (init?.method ?? 'GET').toUpperCase(),
			headers,
			...(typeof init?.body === 'string' ? { body: init.body } : {})
		});
		const body =
			result.status === 204 || result.status === 205 || result.status === 304 ? null : result.body;
		return new Response(body, { status: result.status, headers: result.headers });
	}
}

const transport: ApiTransport = isNativePlatform()
	? new NativeApiTransport()
	: new BrowserApiTransport();

export function apiTransport() {
	return transport;
}
