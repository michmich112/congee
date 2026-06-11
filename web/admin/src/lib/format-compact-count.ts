/** Compact display for large analytics numbers: 999, 1.20K, 10.20M, 1.50B (two decimal places from 1K up). */
export function formatCompactCount(n: number): string {
	if (!Number.isFinite(n)) {
		return '0';
	}
	const sign = n < 0 ? '-' : '';
	const v = Math.abs(n);
	if (v < 1000) {
		return sign + String(Math.round(v));
	}
	if (v < 1_000_000) {
		return sign + (v / 1000).toFixed(2) + 'K';
	}
	if (v < 1_000_000_000) {
		return sign + (v / 1_000_000).toFixed(2) + 'M';
	}
	return sign + (v / 1_000_000_000).toFixed(2) + 'B';
}
