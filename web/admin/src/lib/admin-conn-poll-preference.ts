/** Polling interval (seconds) for Audit → Connections; 0 means off. */
export const LS_ADMIN_CONN_REFRESH_SEC = 'congee-admin-conn-refresh-sec';

export const ADMIN_CONN_REFRESH_ALLOWED = [0, 3, 5, 10, 30, 60] as const;
export type AdminConnRefreshSec = (typeof ADMIN_CONN_REFRESH_ALLOWED)[number];

export function isAdminConnRefreshSec(n: number): n is AdminConnRefreshSec {
	return (ADMIN_CONN_REFRESH_ALLOWED as readonly number[]).includes(n);
}

export function readAdminConnRefreshSecFromStorage(): AdminConnRefreshSec {
	if (typeof localStorage === 'undefined') return 5;
	const raw = localStorage.getItem(LS_ADMIN_CONN_REFRESH_SEC);
	const n = raw ? Number.parseInt(raw, 10) : NaN;
	return isAdminConnRefreshSec(n) ? n : 5;
}

export function writeAdminConnRefreshSecToStorage(sec: AdminConnRefreshSec) {
	localStorage.setItem(LS_ADMIN_CONN_REFRESH_SEC, String(sec));
}
