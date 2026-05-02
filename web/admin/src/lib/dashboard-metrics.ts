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

/** One row per time bin for REQ latency (multi-series chart). */
export type LatencyChartRow = {
	date: Date;
	meanMs: number;
	medianMs: number;
	p99Ms: number;
};

const MIN_MS = 60_000;
const HOUR_SEC = 3600;

function percentileLinear(sorted: number[], p: number): number {
	if (sorted.length === 0) return 0;
	if (sorted.length === 1) return sorted[0];
	const rank = (p / 100) * (sorted.length - 1);
	const lo = Math.floor(rank);
	const hi = Math.ceil(rank);
	if (lo === hi) return sorted[lo];
	return sorted[lo] + (sorted[hi] - sorted[lo]) * (rank - lo);
}

function medianSorted(sorted: number[]): number {
	const n = sorted.length;
	if (n === 0) return 0;
	const mid = Math.floor(n / 2);
	return n % 2 === 1 ? sorted[mid] : (sorted[mid - 1]! + sorted[mid]!) / 2;
}

function latencyStats(values: number[]): { meanMs: number; medianMs: number; p99Ms: number } {
	if (values.length === 0) {
		return { meanMs: 0, medianMs: 0, p99Ms: 0 };
	}
	const sorted = [...values].sort((a, b) => a - b);
	const sum = sorted.reduce((a, b) => a + b, 0);
	return {
		meanMs: sum / sorted.length,
		medianMs: medianSorted(sorted),
		p99Ms: percentileLinear(sorted, 99),
	};
}

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

/** Bin REQ latency samples into rows with mean, median, and p99 per time bucket. */
export function binLatencyChartRows(samples: LatencySample[], resolution: Resolution): LatencyChartRow[] {
	if (!samples.length) return [];
	const sorted = [...samples].sort((a, b) => a.t_unix_ms - b.t_unix_ms);

	function bucketKey(tUnixMs: number): number {
		if (resolution === 'minute') return Math.floor(tUnixMs / MIN_MS) * MIN_MS;
		if (resolution === 'hour')
			return Math.floor(tUnixMs / (HOUR_SEC * 1000)) * HOUR_SEC * 1000;
		return Math.floor(tUnixMs / 1000) * 1000;
	}

	const groups = new Map<number, number[]>();
	for (const s of sorted) {
		const k = bucketKey(s.t_unix_ms);
		let g = groups.get(k);
		if (!g) {
			g = [];
			groups.set(k, g);
		}
		g.push(s.ms);
	}

	return [...groups.entries()]
		.sort(([a], [b]) => a - b)
		.map(([t, arr]) => {
			const st = latencyStats(arr);
			return {
				date: new Date(t),
				meanMs: st.meanMs,
				medianMs: st.medianMs,
				p99Ms: st.p99Ms,
			};
		});
}

export const LS_TIME_RANGE = 'congee.dashboard.timeRange';
export const LS_RESOLUTION = 'congee.dashboard.resolution';
export const LS_REFRESH_SEC = 'congee.dashboard.refreshSec';
