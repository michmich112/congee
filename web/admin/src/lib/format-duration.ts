/** Compact uptime from seconds (e.g. "2d 3h" or "45m"). */
export function formatDurationSec(sec: number): string {
	if (!Number.isFinite(sec) || sec < 0) return '—';
	const s = Math.floor(sec);
	const d = Math.floor(s / 86400);
	const h = Math.floor((s % 86400) / 3600);
	const m = Math.floor((s % 3600) / 60);
	const parts: string[] = [];
	if (d) parts.push(`${d}d`);
	if (h) parts.push(`${h}h`);
	if (!d && !h && m) parts.push(`${m}m`);
	if (!parts.length) parts.push(`${s % 60}s`);
	return parts.join(' ');
}
