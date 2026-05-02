export type Resolution = 'minute' | 'second' | 'hour';

export type TimeRangePreset = '15m' | '1h' | '6h' | '24h';

export const TIME_RANGE_MS: Record<TimeRangePreset, number> = {
	'15m': 15 * 60 * 1000,
	'1h': 60 * 60 * 1000,
	'6h': 6 * 60 * 60 * 1000,
	'24h': 24 * 60 * 60 * 1000,
};

export type MetricBucket = {
	bucket_start_unix: number;
	events_stored: number;
	events_rejected: number;
	req_count: number;
	close_count: number;
	query_ms_avg?: number;
	subscriptions_open?: number;
};

export type LatencySample = { t_unix_ms: number; ms: number };

export type ChartPoint = { date: Date; value: number };

const MIN_MS = 60_000;
const HOUR_SEC = 3600;

export function filterBucketsByRange(buckets: MetricBucket[], rangeMs: number, now = Date.now()): MetricBucket[] {
	const cutoff = now - rangeMs;
	return buckets.filter((b) => b.bucket_start_unix * 1000 >= cutoff);
}

export function filterLatencyByRange(samples: LatencySample[], rangeMs: number, now = Date.now()): LatencySample[] {
	const cutoff = now - rangeMs;
	return samples.filter((s) => s.t_unix_ms >= cutoff);
}

/** Roll minute buckets into UTC hour bins (sums counts; last subscriptions_open in hour). */
export function rollupBucketsHourly(buckets: MetricBucket[]): MetricBucket[] {
	if (!buckets.length) return [];
	const sorted = [...buckets].sort((a, b) => a.bucket_start_unix - b.bucket_start_unix);
	const out: MetricBucket[] = [];
	let curHour = Math.floor(sorted[0].bucket_start_unix / HOUR_SEC) * HOUR_SEC;
	let acc: MetricBucket = {
		bucket_start_unix: curHour,
		events_stored: 0,
		events_rejected: 0,
		req_count: 0,
		close_count: 0,
		subscriptions_open: sorted[0].subscriptions_open ?? 0,
	};
	const flush = () => {
		out.push({ ...acc });
	};
	for (const b of sorted) {
		const hourStart = Math.floor(b.bucket_start_unix / HOUR_SEC) * HOUR_SEC;
		if (hourStart !== curHour) {
			flush();
			curHour = hourStart;
			acc = {
				bucket_start_unix: curHour,
				events_stored: 0,
				events_rejected: 0,
				req_count: 0,
				close_count: 0,
				subscriptions_open: b.subscriptions_open ?? 0,
			};
		}
		acc.events_stored += b.events_stored;
		acc.events_rejected += b.events_rejected;
		acc.req_count += b.req_count;
		acc.close_count += b.close_count;
		acc.subscriptions_open = b.subscriptions_open ?? acc.subscriptions_open;
	}
	flush();
	return out;
}

function ratePerSecond(n: number): number {
	return n / 60;
}

/** Events / REQ / close counts: optional avg-per-second view within each minute. Subscriptions stay snapshot per minute. */
export function bucketSeries(
	buckets: MetricBucket[],
	field: 'events_stored' | 'req_count' | 'subscriptions_open',
	resolution: Resolution
): ChartPoint[] {
	let rows = buckets;
	if (resolution === 'hour') {
		rows = rollupBucketsHourly(buckets);
	}
	return rows.map((b) => {
		const raw = Number(b[field] ?? 0);
		let value = raw;
		if (resolution === 'second') {
			value = field === 'subscriptions_open' ? raw : ratePerSecond(raw);
		}
		return {
			date: new Date(b.bucket_start_unix * 1000),
			value,
		};
	});
}

export function binLatencySamples(samples: LatencySample[], resolution: Resolution): ChartPoint[] {
	if (!samples.length) return [];
	const sorted = [...samples].sort((a, b) => a.t_unix_ms - b.t_unix_ms);

	if (resolution === 'minute') {
		const groups = new Map<number, number[]>();
		for (const s of sorted) {
			const k = Math.floor(s.t_unix_ms / MIN_MS) * MIN_MS;
			let g = groups.get(k);
			if (!g) {
				g = [];
				groups.set(k, g);
			}
			g.push(s.ms);
		}
		return [...groups.entries()]
			.sort(([a], [b]) => a - b)
			.map(([t, arr]) => ({
				date: new Date(t),
				value: arr.reduce((x, y) => x + y, 0) / arr.length,
			}));
	}

	if (resolution === 'hour') {
		const groups = new Map<number, number[]>();
		for (const s of sorted) {
			const k = Math.floor(s.t_unix_ms / (HOUR_SEC * 1000)) * HOUR_SEC * 1000;
			let g = groups.get(k);
			if (!g) {
				g = [];
				groups.set(k, g);
			}
			g.push(s.ms);
		}
		return [...groups.entries()]
			.sort(([a], [b]) => a - b)
			.map(([t, arr]) => ({
				date: new Date(t),
				value: arr.reduce((x, y) => x + y, 0) / arr.length,
			}));
	}

	// second
	const groups = new Map<number, number[]>();
	for (const s of sorted) {
		const k = Math.floor(s.t_unix_ms / 1000) * 1000;
		let g = groups.get(k);
		if (!g) {
			g = [];
			groups.set(k, g);
		}
		g.push(s.ms);
	}
	return [...groups.entries()]
		.sort(([a], [b]) => a - b)
		.map(([t, arr]) => ({
			date: new Date(t),
			value: arr.reduce((x, y) => x + y, 0) / arr.length,
		}));
}

export const LS_TIME_RANGE = 'congee.dashboard.timeRange';
export const LS_RESOLUTION = 'congee.dashboard.resolution';
export const LS_REFRESH_SEC = 'congee.dashboard.refreshSec';
