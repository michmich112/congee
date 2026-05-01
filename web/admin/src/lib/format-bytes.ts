/** Human-readable byte size (binary units). */
export function formatBytes(n: number): string {
	if (!Number.isFinite(n) || n < 0) return '—';
	if (n < 1024) return `${Math.round(n)} B`;
	const units = ['KiB', 'MiB', 'GiB', 'TiB'];
	let v = n / 1024;
	let i = 0;
	while (v >= 1024 && i < units.length - 1) {
		v /= 1024;
		i++;
	}
	const digits = v >= 100 ? 0 : v >= 10 ? 1 : 2;
	return `${v.toFixed(digits)} ${units[i]}`;
}
