import { browser } from '$app/environment';
import type { TimestampDisplayMode } from '$lib/format-timestamp';

const STORAGE_KEY = 'congee-admin-timestamp-display';

/** Shared table timestamp mode; mutate `.mode` (assigning the object would break reactivity). */
export const timestampDisplay = $state<{ mode: TimestampDisplayMode }>({ mode: 'unix' });

export function initTimestampDisplayFromStorage(): void {
	if (!browser) return;
	const raw = localStorage.getItem(STORAGE_KEY);
	if (raw === 'utc' || raw === 'local' || raw === 'unix') {
		timestampDisplay.mode = raw;
	}
}

export function setTimestampDisplayMode(mode: TimestampDisplayMode): void {
	timestampDisplay.mode = mode;
	if (browser) localStorage.setItem(STORAGE_KEY, mode);
}
