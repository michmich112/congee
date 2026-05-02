export type Resolution = 'minute' | 'hour';

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
	query_ms_sum?: number;
	query_ms_count?: number;
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

/** UTC hour bins with summed counters and weighted REQ latency (sum/count). */
export function rollupBucketsHourlyWithLatency(buckets: MetricBucket[]): MetricBucket[] {
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
		query_ms_sum: 0,
		query_ms_count: 0,
		subscriptions_open: sorted[0].subscriptions_open ?? 0,
	};
	const flush = () => {
		const qCnt = Number(acc.query_ms_count ?? 0);
		const qSum = Number(acc.query_ms_sum ?? 0);
		const row: MetricBucket = {
			bucket_start_unix: acc.bucket_start_unix,
			events_stored: acc.events_stored,
			events_rejected: acc.events_rejected,
			req_count: acc.req_count,
			close_count: acc.close_count,
			subscriptions_open: acc.subscriptions_open,
		};
		if (qCnt > 0) {
			row.query_ms_sum = qSum;
			row.query_ms_count = qCnt;
			row.query_ms_avg = qSum / qCnt;
		}
		out.push(row);
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
				query_ms_sum: 0,
				query_ms_count: 0,
				subscriptions_open: b.subscriptions_open ?? 0,
			};
		}
		acc.events_stored += b.events_stored;
		acc.events_rejected += b.events_rejected;
		acc.req_count += b.req_count;
		acc.close_count += b.close_count;
		acc.query_ms_sum = (acc.query_ms_sum ?? 0) + Number(b.query_ms_sum ?? 0);
		acc.query_ms_count = (acc.query_ms_count ?? 0) + Number(b.query_ms_count ?? 0);
		acc.subscriptions_open = b.subscriptions_open ?? acc.subscriptions_open;
	}
	flush();
	return out;
}

/** Events / REQ / close counts from minute buckets (per-minute or rolled up per hour). */
export function bucketSeries(
	buckets: MetricBucket[],
	field: 'events_stored' | 'req_count' | 'subscriptions_open',
	resolution: Resolution
): ChartPoint[] {
	let rows = buckets;
	if (resolution === 'hour') {
		rows = rollupBucketsHourly(buckets);
	}
	return rows.map((b) => ({
		date: new Date(b.bucket_start_unix * 1000),
		value: Number(b[field] ?? 0),
	}));
}

/**
 * REQ latency rows aligned to persisted metric buckets (same retention as other charts).
 * Uses recent in-memory samples for mean/median/p99 when present; otherwise falls back to
 * per-bucket average latency from storage (median/p99 equal mean in that case).
 */
export function latencyChartRowsFromBucketsAndSamples(
	buckets: MetricBucket[],
	samples: LatencySample[],
	resolution: Resolution,
	rangeMs: number,
	now = Date.now()
): LatencyChartRow[] {
	const fb = filterBucketsByRange(buckets, rangeMs, now);
	const fs = filterLatencyByRange(samples, rangeMs, now);

	let timeline = fb;
	if (resolution === 'hour') {
		timeline = rollupBucketsHourlyWithLatency(fb);
	}

	const sampleBins = new Map<number, number[]>();
	for (const s of fs) {
		const k =
			resolution === 'minute'
				? Math.floor(s.t_unix_ms / MIN_MS) * MIN_MS
				: Math.floor(s.t_unix_ms / (HOUR_SEC * 1000)) * HOUR_SEC * 1000;
		let arr = sampleBins.get(k);
		if (!arr) {
			arr = [];
			sampleBins.set(k, arr);
		}
		arr.push(s.ms);
	}

	const sorted = [...timeline].sort((a, b) => a.bucket_start_unix - b.bucket_start_unix);
	const rows: LatencyChartRow[] = [];

	for (const b of sorted) {
		const tMs = b.bucket_start_unix * 1000;
		const key =
			resolution === 'minute'
				? Math.floor(tMs / MIN_MS) * MIN_MS
				: Math.floor(tMs / (HOUR_SEC * 1000)) * HOUR_SEC * 1000;

		const binSamples = sampleBins.get(key);
		if (binSamples?.length) {
			const st = latencyStats(binSamples);
			rows.push({
				date: new Date(key),
				meanMs: st.meanMs,
				medianMs: st.medianMs,
				p99Ms: st.p99Ms,
			});
			continue;
		}

		const qCnt = Number(b.query_ms_count ?? 0);
		const qSum = Number(b.query_ms_sum ?? 0);
		if (qCnt > 0) {
			const avg = qSum / qCnt;
			rows.push({
				date: new Date(key),
				meanMs: avg,
				medianMs: avg,
				p99Ms: avg,
			});
			continue;
		}

		const avgOnly = b.query_ms_avg;
		if (typeof avgOnly === 'number' && Number.isFinite(avgOnly) && avgOnly > 0) {
			rows.push({
				date: new Date(key),
				meanMs: avgOnly,
				medianMs: avgOnly,
				p99Ms: avgOnly,
			});
		}
	}

	return rows;
}

export const LS_TIME_RANGE = 'congee.dashboard.timeRange';
export const LS_RESOLUTION = 'congee.dashboard.resolution';
export const LS_REFRESH_SEC = 'congee.dashboard.refreshSec';
