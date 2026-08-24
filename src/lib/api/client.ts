import type { APIError } from './types';

export class XiviAPIError extends Error {
	constructor(
		public status: number,
		public detail: APIError
	) {
		super(detail.message);
	}
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
	const response = await fetch(path, {
		...init,
		headers: {
			Accept: 'application/json',
			...(init?.body ? { 'Content-Type': 'application/json' } : {}),
			...init?.headers
		}
	});
	if (!response.ok) {
		let detail: APIError = {
			code: 'request_failed',
			message: 'Xivi could not complete that request.',
			retryable: response.status >= 500
		};
		try {
			detail = await response.json();
		} catch {
			/* response was not JSON */
		}
		throw new XiviAPIError(response.status, detail);
	}
	if (response.status === 204) return undefined as T;
	return response.json() as Promise<T>;
}

export function params(values: Record<string, string | number | boolean | undefined | null>) {
	const search = new URLSearchParams();
	for (const [key, value] of Object.entries(values))
		if (value !== undefined && value !== null && value !== '') search.set(key, String(value));
	const result = search.toString();
	return result ? `?${result}` : '';
}
