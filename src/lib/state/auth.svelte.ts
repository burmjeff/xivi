import { browser } from '$app/environment';

export interface SessionPrincipal {
	user_id: number;
	username: string;
	display_name: string;
	role: 'admin' | 'viewer';
	must_change_password: boolean;
	mfa_enabled: boolean;
	mfa_required: boolean;
	lineup_ids: number[];
	csrf_token: string;
	security_notice?: {
		kind: string;
		count: number;
		last_at: string;
		last_ip?: string;
		message: string;
	};
}

type AuthStatus = 'loading' | 'authenticated' | 'anonymous';

export const auth = $state<{
	status: AuthStatus;
	principal: SessionPrincipal | null;
	bootstrapRequired: boolean;
	initialPasswordChangeRequired: boolean;
	initialLoginAllowed: boolean;
}>({
	status: 'loading',
	principal: null,
	bootstrapRequired: false,
	initialPasswordChangeRequired: false,
	initialLoginAllowed: false
});

export function setSession(principal: SessionPrincipal | null) {
	auth.principal = principal;
	auth.status = principal ? 'authenticated' : 'anonymous';
	if (principal) auth.bootstrapRequired = false;
	if (principal && !principal.must_change_password) {
		auth.initialPasswordChangeRequired = false;
		auth.initialLoginAllowed = false;
	}
}

export function clearSession() {
	setSession(null);
}

export function csrfToken() {
	return auth.principal?.csrf_token ?? '';
}

async function requestSession(
	checkBootstrap: boolean,
	preserveOnFailure: boolean,
	passive = false
) {
	if (!browser) return;
	try {
		const response = await fetch('/api/v2/auth/session', {
			credentials: 'same-origin',
			headers: {
				Accept: 'application/json',
				...(passive ? { 'X-Xivi-Session-Check': 'passive' } : {})
			},
			cache: 'no-store'
		});
		if (response.ok) {
			setSession((await response.json()) as SessionPrincipal);
			return;
		}
		if (response.status === 401) {
			if (checkBootstrap) {
				const bootstrapResponse = await fetch('/api/v2/auth/bootstrap-status', {
					credentials: 'same-origin',
					headers: { Accept: 'application/json' },
					cache: 'no-store'
				});
				if (bootstrapResponse.ok) {
					const bootstrap = (await bootstrapResponse.json()) as {
						bootstrap_required: boolean;
						initial_password_change_required: boolean;
						initial_login_allowed: boolean;
					};
					auth.bootstrapRequired = bootstrap.bootstrap_required;
					auth.initialPasswordChangeRequired = bootstrap.initial_password_change_required;
					auth.initialLoginAllowed = bootstrap.initial_login_allowed;
				}
			}
			setSession(null);
			return;
		}
		if (preserveOnFailure) return;
	} catch {
		// A transient network failure must not erase a still-valid session. API
		// requests remain deny-by-default on the server, and the next focus/poll
		// check retries validation.
		if (preserveOnFailure) return;
	}
	setSession(null);
}

export async function loadSession() {
	if (!browser) return;
	auth.status = 'loading';
	await requestSession(true, false);
}

export async function refreshSession(passive = false) {
	if (!browser || auth.status !== 'authenticated') return;
	await requestSession(false, true, passive);
}
