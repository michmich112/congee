const STORAGE_KEY = 'congee_admin_token';

export function getAdminToken(): string | null {
	if (typeof localStorage === 'undefined') return null;
	return localStorage.getItem(STORAGE_KEY);
}

export function setAdminToken(token: string): void {
	localStorage.setItem(STORAGE_KEY, token);
}

export function clearAdminToken(): void {
	localStorage.removeItem(STORAGE_KEY);
}

/** GET/POST etc. with `Authorization: Bearer` from stored token (if any). */
export function adminFetch(input: string, init: RequestInit = {}): Promise<Response> {
	const headers = new Headers(init.headers);
	const t = getAdminToken();
	if (t) headers.set('Authorization', `Bearer ${t}`);
	return fetch(input, { ...init, headers });
}

export function adminFetchWithToken(input: string, token: string, init: RequestInit = {}): Promise<Response> {
	const headers = new Headers(init.headers);
	headers.set('Authorization', `Bearer ${token}`);
	return fetch(input, { ...init, headers });
}

/** True if this password is accepted by `GET /api/stats`. */
export async function verifyAdminToken(token: string): Promise<boolean> {
	const r = await adminFetchWithToken('/api/stats', token);
	return r.ok;
}
