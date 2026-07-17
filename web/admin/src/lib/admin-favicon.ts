import { browser } from '$app/environment';

const FAVICON_LIGHT = '/favicon-light.svg';
const FAVICON_DARK = '/favicon-dark.svg';
const MODE_STORAGE_KEY = 'mode-watcher-mode';

export function resolveAdminFaviconIsDark(): boolean {
	if (!browser) return false;

	const stored = localStorage.getItem(MODE_STORAGE_KEY);
	if (stored === 'dark') return true;
	if (stored === 'light') return false;

	return (
		document.documentElement.classList.contains('dark') ||
		window.matchMedia('(prefers-color-scheme: dark)').matches
	);
}

export function syncAdminFavicon(isDark = resolveAdminFaviconIsDark()): void {
	if (!browser) return;

	const href = isDark ? FAVICON_DARK : FAVICON_LIGHT;
	const existing = document.querySelector<HTMLLinkElement>('link[data-congee-favicon]');

	if (existing && new URL(existing.href, location.origin).pathname === href) return;

	document.querySelectorAll('link[rel="icon"]').forEach((el) => el.remove());

	const link = document.createElement('link');
	link.rel = 'icon';
	link.type = 'image/svg+xml';
	link.href = href;
	link.dataset.congeeFavicon = 'true';
	document.head.appendChild(link);
}
