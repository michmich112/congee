export type TimestampDisplayMode = 'unix' | 'utc' | 'local';

/** Backend stores Unix seconds; values ≥ 1e12 are treated as Unix milliseconds. */
export function toEpochMs(stored: number): number {
	return stored >= 1_000_000_000_000 ? stored : stored * 1000;
}

function pad(n: number, w: number): string {
	return String(n).padStart(w, '0');
}

function wallClock(d: Date, zone: 'utc' | 'local'): string {
	const Y = zone === 'utc' ? d.getUTCFullYear() : d.getFullYear();
	const M = pad(zone === 'utc' ? d.getUTCMonth() + 1 : d.getMonth() + 1, 2);
	const D = pad(zone === 'utc' ? d.getUTCDate() : d.getDate(), 2);
	const h = pad(zone === 'utc' ? d.getUTCHours() : d.getHours(), 2);
	const m = pad(zone === 'utc' ? d.getUTCMinutes() : d.getMinutes(), 2);
	const s = pad(zone === 'utc' ? d.getUTCSeconds() : d.getSeconds(), 2);
	const f = pad(zone === 'utc' ? d.getUTCMilliseconds() : d.getMilliseconds(), 3);
	return `${Y}-${M}-${D} ${h}:${m}:${s}.${f}`;
}

export function formatTimestamp(mode: TimestampDisplayMode, storedUnix: number): string {
	const ms = toEpochMs(storedUnix);
	if (mode === 'unix') return String(ms);
	const d = new Date(ms);
	if (mode === 'utc') return `${wallClock(d, 'utc')} UTC`;
	const tz = Intl.DateTimeFormat().resolvedOptions().timeZone;
	return `${wallClock(d, 'local')} (${tz})`;
}
