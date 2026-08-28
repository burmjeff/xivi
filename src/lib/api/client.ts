import type { APIError } from './types';
import { clearSession, csrfToken } from '$lib/state/auth.svelte';

export class XiviAPIError extends Error {
	constructor(
		public status: number,
		public detail: APIError
	) {
		super(detail.message);
	}
}

const sessionInvalidatingCodes = new Set([
	'authentication_required',
	'session_expired',
	'session_revoked'
]);

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
	const method = (init?.method ?? 'GET').toUpperCase();
	const mutation = !['GET', 'HEAD', 'OPTIONS'].includes(method);
	const response = await fetch(path, {
		...init,
		credentials: 'same-origin',
		cache: 'no-store',
		headers: {
			Accept: 'application/json',
			...(init?.body ? { 'Content-Type': 'application/json' } : {}),
			...(mutation && csrfToken() ? { 'X-CSRF-Token': csrfToken() } : {}),
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
			const parsed = (await response.json()) as Partial<APIError> & { msg?: unknown };
			detail = {
				code: typeof parsed.code === 'string' ? parsed.code : detail.code,
				message:
					typeof parsed.message === 'string'
						? parsed.message
						: typeof parsed.msg === 'string'
							? parsed.msg
							: detail.message,
				retryable: typeof parsed.retryable === 'boolean' ? parsed.retryable : detail.retryable,
				...(parsed.field_errors ? { field_errors: parsed.field_errors } : {}),
				...(parsed.challenge ? { challenge: parsed.challenge } : {})
			};
		} catch {
			/* response was not JSON */
		}
		// A failed password confirmation is a credential error for that one
		// operation, not evidence that the existing browser session is invalid.
		// Only explicit session-authentication errors should redirect to login.
		if (response.status === 401 && sessionInvalidatingCodes.has(detail.code)) clearSession();
		throw new XiviAPIError(response.status, detail);
	}
	if (response.status === 204 || response.headers.get('content-length') === '0')
		return undefined as T;
	const contentType = response.headers.get('content-type') ?? '';
	if (contentType.includes('json')) return response.json() as Promise<T>;
	const text = await response.text();
	return (text || undefined) as T;
}

export function params(values: Record<string, string | number | boolean | undefined | null>) {
	const search = new URLSearchParams();
	for (const [key, value] of Object.entries(values))
		if (value !== undefined && value !== null && value !== '') search.set(key, String(value));
	const result = search.toString();
	return result ? `?${result}` : '';
}
