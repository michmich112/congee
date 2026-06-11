<script lang="ts">
	import { onMount } from 'svelte';
	import { browser } from '$app/environment';
	import LayoutDashboard from '@lucide/svelte/icons/layout-dashboard';
	import Loader2Icon from '@lucide/svelte/icons/loader-2';
	import { adminFetch } from '$lib/admin-api';
	import { formatBytes } from '$lib/format-bytes';
	import { formatDurationSec } from '$lib/format-duration';
	import { formatCompactCount } from '$lib/format-compact-count';
	import {
		LS_REFRESH_SEC,
		LS_RESOLUTION,
		LS_TIME_RANGE,
		TIME_RANGE_MS,
		latencyChartRowsFromBucketsAndSamples,
		bucketSeries,
		filterBucketsByRange,
		type LatencySample,
		type MetricBucket,
		type Resolution,
		type TimeRangePreset
	} from '$lib/dashboard-metrics';
	import AdminPageHeading from '$lib/components/AdminPageHeading.svelte';
	import AnalyticsStatCard from '$lib/components/AnalyticsStatCard.svelte';
	import DashboardGraphCard from '$lib/components/DashboardGraphCard.svelte';
	import DashboardLatencyChart from '$lib/components/DashboardLatencyChart.svelte';
	import DashboardLineSeriesChart from '$lib/components/DashboardLineSeriesChart.svelte';
	import StatInfoIcon from '$lib/components/StatInfoIcon.svelte';
	import * as Card from '$lib/components/ui/card';
	import { Button } from '$lib/components/ui/button';
	import { Label } from '$lib/components/ui/label';

	const selectClass =
		'border-input bg-background focus-visible:ring-ring/50 h-9 min-w-[8rem] rounded-md border px-2.5 text-sm shadow-xs focus-visible:ring-2 focus-visible:outline-none';

	/** Shown in the (i) tooltip next to each dashboard chart title. */
	const DASHBOARD_CHART_INFO = {
		eventsStored:
			'Nostr events this relay accepted and stored, per time bucket. Per minute: each UTC minute. Per hour: sums of those minutes in each UTC hour.',
		reqCount:
			'How many incoming REQ (subscription) messages the relay handled in each bucket. Per hour adds the per-minute counts in that hour.',
		latency:
			'How long REQ query work took. With enough recent samples, lines are mean, median, and 99th percentile; otherwise the chart uses stored per-minute averages (the three series overlap as the same value).',
		subs:
			'How many open REQ filter subscriptions the relay is serving. Each point is a snapshot: per minute the value at the end of that minute; per hour the last sample in the hour.',
	} as const;

	/** Hover tooltips for dashboard metric tiles (first rows + rate limits). */
	const DASHBOARD_METRIC_INFO = {
		ws: 'Current WebSocket connections to this relay process.',
		subs: 'Open REQ filter subscriptions across all connections.',
		uptime: 'Time since this relay process started serving, and local wall-clock start time.',
		storage: 'Combined on-disk size of the events and meta SQLite databases. Hover the (i) icon for per-database size and row counts.',
		eventsStored:
			'Events that passed validation and were persisted (lifetime counter since process start).',
		ephemeral:
			'Ephemeral event kinds (NIP-01): accepted with OK, not written to the database, still broadcast to subscribers. Excludes rejected events.',
		eventsRejected:
			'Events that failed validation or storage (lifetime counter since process start).',
		reqClose:
			'Total REQ subscription requests and CLOSE messages handled since process start (shown as REQ / CLOSE).',
		rateLimits:
			'How often per-connection or per-IP limits blocked traffic (cumulative since process start).',
	} as const;

	type Stats = {
		open_connections?: number;
		subscriptions_open?: number;
		started_at_unix?: number;
		uptime_sec?: number;
		relay_counters?: Record<string, number>;
		recent_query_latency?: LatencySample[];
		storage?: {
			bytes?: number;
			meta_bytes?: number;
			events?: number;
			tags?: number;
			audit?: number;
		};
		series?: { bucket_sec?: number; buckets?: MetricBucket[] };
	};

	let stats = $state<Stats | null>(null);
	let loadErr = $state<string | null>(null);
	let loading = $state(true);
	let pollErr = $state<string | null>(null);
	let refreshBusy = $state(false);
	let lastUpdated = $state<number | null>(null);

	let timeRange = $state<TimeRangePreset>('1h');
	let resolution = $state<Resolution>('minute');
	let refreshSec = $state(5);

	onMount(async () => {
		const tr = localStorage.getItem(LS_TIME_RANGE);
		if (tr === '15m' || tr === '1h' || tr === '6h' || tr === '24h') timeRange = tr;
		const res = localStorage.getItem(LS_RESOLUTION);
		if (res === 'minute' || res === 'hour') resolution = res;
		if (res === 'second') resolution = 'minute';
		const rs = localStorage.getItem(LS_REFRESH_SEC);
		const rv = rs ? Number(rs) : NaN;
		if ([3, 5, 10, 30, 60].includes(rv)) refreshSec = rv;

		try {
			const statsRes = await adminFetch('/api/stats');
			if (!statsRes.ok) {
				loadErr = statsRes.status === 401 ? 'Unauthorized' : `HTTP ${statsRes.status}`;
				return;
			}
			stats = (await statsRes.json()) as Stats;
			lastUpdated = Date.now();
		} catch (e) {
			loadErr = e instanceof Error ? e.message : 'request failed';
		} finally {
			loading = false;
		}
	});

	$effect(() => {
		if (!browser) return;
		localStorage.setItem(LS_TIME_RANGE, timeRange);
		localStorage.setItem(LS_RESOLUTION, resolution);
		localStorage.setItem(LS_REFRESH_SEC, String(refreshSec));
	});

	async function refreshStats(manual = false) {
		if (manual) refreshBusy = true;
		try {
			const statsRes = await adminFetch('/api/stats');
			if (!statsRes.ok) {
				pollErr = statsRes.status === 401 ? 'Unauthorized' : `HTTP ${statsRes.status}`;
				return;
			}
			pollErr = null;
			stats = (await statsRes.json()) as Stats;
			lastUpdated = Date.now();
		} catch (e) {
			pollErr = e instanceof Error ? e.message : 'request failed';
		} finally {
			if (manual) refreshBusy = false;
		}
	}

	$effect(() => {
		if (!browser || loading) return;
		const ms = refreshSec * 1000;
		const id = window.setInterval(() => {
			void refreshStats(false);
		}, ms);
		return () => window.clearInterval(id);
	});

	function counter(name: string): number {
		const v = stats?.relay_counters?.[name];
		return typeof v === 'number' && Number.isFinite(v) ? v : 0;
	}

	const rangeMs = $derived(TIME_RANGE_MS[timeRange]);

	const filteredBuckets = $derived.by((): MetricBucket[] => {
		const b = stats?.series?.buckets;
		if (!b?.length) return [];
		return filterBucketsByRange(b, rangeMs);
	});

	const eventsChartData = $derived.by(() =>
		bucketSeries(filteredBuckets, 'events_stored', resolution));

	const reqChartData = $derived.by(() => bucketSeries(filteredBuckets, 'req_count', resolution));

	const subsChartData = $derived.by(() =>
		bucketSeries(filteredBuckets, 'subscriptions_open', resolution));

	const latencyChartData = $derived.by(() => {
		const buckets = stats?.series?.buckets ?? [];
		const samples = stats?.recent_query_latency ?? [];
		return latencyChartRowsFromBucketsAndSamples(buckets, samples, resolution, rangeMs);
	});

</script>

<div class="space-y-6">
	<AdminPageHeading
		title="Dashboard"
		subtitle="Live relay health, traffic, and storage."
		Icon={LayoutDashboard}
	/>

	{#if loading}
		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
			{#each Array(8) as _, i (i)}
				<div class="bg-muted/50 h-24 animate-pulse rounded-lg"></div>
			{/each}
		</div>
		<p class="text-muted-foreground text-sm">Loading stats…</p>
	{:else if loadErr}
		<p class="text-destructive text-sm">{loadErr}</p>
	{:else if stats}
		<div class="text-muted-foreground flex flex-wrap items-center gap-3 text-xs">
			{#if lastUpdated}
				<span>Updated {new Date(lastUpdated).toLocaleTimeString()}</span>
			{/if}
			{#if pollErr}
				<span class="text-destructive">Poll failed: {pollErr}</span>
			{:else}
				<span>Refreshing every {refreshSec}s</span>
			{/if}
		</div>

		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
			<AnalyticsStatCard label="WebSocket connections" value={stats.open_connections ?? 0} info={DASHBOARD_METRIC_INFO.ws} />
			<AnalyticsStatCard label="Subscriptions" value={stats.subscriptions_open ?? 0} info={DASHBOARD_METRIC_INFO.subs} />
			<Card.Root>
				<Card.Header class="pb-2">
					<div class="flex flex-wrap items-center gap-x-2 gap-y-1">
						<Card.Description>Uptime</Card.Description>
						<StatInfoIcon info={DASHBOARD_METRIC_INFO.uptime} />
					</div>
					<Card.Title class="text-xl tabular-nums leading-snug">
						{formatDurationSec(stats.uptime_sec ?? 0)}
					</Card.Title>
				</Card.Header>
				<Card.Content class="text-muted-foreground text-xs">
					{#if stats.started_at_unix}
						Started {new Date((stats.started_at_unix ?? 0) * 1000).toLocaleString()}
					{:else}
						Relay not serving yet
					{/if}
				</Card.Content>
			</Card.Root>
			<Card.Root>
				<Card.Header class="pb-2">
					<div class="flex flex-wrap items-center gap-x-2 gap-y-1">
						<Card.Description>Storage (DB)</Card.Description>
						<StatInfoIcon info={DASHBOARD_METRIC_INFO.storage}>
							{#snippet content()}
								<table class="w-full border-collapse text-xs">
									<thead>
										<tr class="text-muted-foreground border-border border-b">
											<th class="pr-4 pb-1.5 text-left font-medium"></th>
											<th class="pr-4 pb-1.5 text-right font-medium">Size</th>
											<th class="pb-1.5 text-right font-medium">Rows</th>
										</tr>
									</thead>
									<tbody class="text-popover-foreground">
										<tr>
											<td class="pr-4 pt-1.5 font-medium whitespace-nowrap">Events DB</td>
											<td class="pr-4 pt-1.5 text-right tabular-nums whitespace-nowrap">
												{formatBytes(stats.storage?.bytes ?? 0)}
											</td>
											<td class="pt-1.5 text-right tabular-nums whitespace-nowrap">
												{formatCompactCount(stats.storage?.events ?? 0)} events ·
												{formatCompactCount(stats.storage?.tags ?? 0)} tags
											</td>
										</tr>
										<tr>
											<td class="pr-4 pt-1.5 font-medium whitespace-nowrap">Meta DB</td>
											<td class="pr-4 pt-1.5 text-right tabular-nums whitespace-nowrap">
												{formatBytes(stats.storage?.meta_bytes ?? 0)}
											</td>
											<td class="pt-1.5 text-right tabular-nums whitespace-nowrap">
												{formatCompactCount(stats.storage?.audit ?? 0)} audit
											</td>
										</tr>
									</tbody>
								</table>
							{/snippet}
						</StatInfoIcon>
					</div>
					<Card.Title class="text-xl tabular-nums leading-snug">
						{formatBytes((stats.storage?.bytes ?? 0) + (stats.storage?.meta_bytes ?? 0))}
					</Card.Title>
				</Card.Header>
			</Card.Root>
		</div>

		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
			<AnalyticsStatCard
				label="Events stored (ok)"
				value={counter('events_stored_ok')}
				compactNumber
				info={DASHBOARD_METRIC_INFO.eventsStored}
			/>
			<AnalyticsStatCard
				label="Ephemeral processed (ok)"
				value={counter('events_ephemeral_ok')}
				compactNumber
				info={DASHBOARD_METRIC_INFO.ephemeral}
			/>
			<AnalyticsStatCard
				label="Events rejected"
				value={counter('events_rejected')}
				compactNumber
				info={DASHBOARD_METRIC_INFO.eventsRejected}
			/>
			<Card.Root>
				<Card.Header class="pb-2">
					<div class="flex flex-wrap items-center gap-x-2 gap-y-1">
						<Card.Description>REQ / CLOSE (total)</Card.Description>
						<StatInfoIcon info={DASHBOARD_METRIC_INFO.reqClose} />
					</div>
					<Card.Title class="text-3xl tabular-nums">
						{formatCompactCount(counter('req_total'))} / {formatCompactCount(counter('close_total'))}
					</Card.Title>
				</Card.Header>
			</Card.Root>
		</div>

		<Card.Root>
			<Card.Header class="pb-2">
				<div class="flex flex-wrap items-start gap-x-2 gap-y-1">
					<div class="grid min-w-0 flex-1 gap-0.5">
						<Card.Title class="text-base">Rate limits (cumulative)</Card.Title>
						<Card.Description>Hits since relay process start</Card.Description>
					</div>
					<StatInfoIcon info={DASHBOARD_METRIC_INFO.rateLimits} />
				</div>
			</Card.Header>
			<Card.Content class="grid gap-2 text-sm sm:grid-cols-2 lg:grid-cols-3">
				<div>
					Messages / IP: <span class="font-mono">{formatCompactCount(counter('rate_limit_messages'))}</span>
				</div>
				<div>
					Bandwidth: <span class="font-mono">{formatCompactCount(counter('rate_limit_bandwidth'))}</span>
				</div>
				<div>
					Events / conn: <span class="font-mono">{formatCompactCount(counter('rate_limit_events'))}</span>
				</div>
				<div>
					REQ / conn: <span class="font-mono">{formatCompactCount(counter('rate_limit_reqs'))}</span>
				</div>
				<div>
					New conn / IP: <span class="font-mono">{formatCompactCount(counter('rate_limit_new_connections'))}</span>
				</div>
				<div>
					Max connections: <span class="font-mono">{formatCompactCount(counter('rate_limit_max_connections'))}</span>
				</div>
				<div>
					Open / IP: <span class="font-mono">{formatCompactCount(counter('rate_limit_per_ip_open'))}</span>
				</div>
				<div>
					Idle disconnects: <span class="font-mono">{formatCompactCount(counter('idle_disconnect_total'))}</span>
				</div>
			</Card.Content>
		</Card.Root>

		<div
			class="border-border bg-muted/30 flex flex-col gap-4 rounded-lg border p-4 sm:flex-row sm:items-end sm:justify-between"
			aria-label="Chart range and refresh options"
		>
			<div class="flex flex-wrap items-end gap-4">
				<div class="grid gap-1.5">
					<Label for="dash-time-range">Time range</Label>
					<select id="dash-time-range" class={selectClass} bind:value={timeRange}>
						<option value="15m">Last 15 minutes</option>
						<option value="1h">Last hour</option>
						<option value="6h">Last 6 hours</option>
						<option value="24h">Last 24 hours</option>
					</select>
				</div>
				<div class="grid gap-1.5">
					<Label for="dash-resolution">Resolution</Label>
					<select id="dash-resolution" class={selectClass} bind:value={resolution}>
						<option value="minute">Per minute</option>
						<option value="hour">Per hour</option>
					</select>
				</div>
				<div class="grid gap-1.5">
					<Label for="dash-refresh">Refresh</Label>
					<select id="dash-refresh" class={selectClass} bind:value={refreshSec}>
						<option value={3}>3s</option>
						<option value={5}>5s</option>
						<option value={10}>10s</option>
						<option value={30}>30s</option>
						<option value={60}>1m</option>
					</select>
				</div>
			</div>
			<div class="flex justify-end sm:shrink-0">
				<Button
					type="button"
					variant="secondary"
					size="sm"
					class="h-9"
					disabled={refreshBusy}
					aria-busy={refreshBusy}
					onclick={() => void refreshStats(true)}
				>
					{#if refreshBusy}
						<Loader2Icon class="size-3.5 animate-spin" aria-hidden="true" />
						<span>Refreshing…</span>
					{:else}
						Refresh now
					{/if}
				</Button>
			</div>
		</div>

		<div class="grid gap-4 lg:grid-cols-2">
			<DashboardGraphCard title="Events stored" info={DASHBOARD_CHART_INFO.eventsStored}>
				<DashboardLineSeriesChart
					data={eventsChartData}
					seriesLabel="Events"
					color="var(--chart-1)"
					seriesKey="events"
				/>
			</DashboardGraphCard>
			<DashboardGraphCard title="REQ count" info={DASHBOARD_CHART_INFO.reqCount}>
				<DashboardLineSeriesChart
					data={reqChartData}
					seriesLabel="REQ"
					color="var(--chart-2)"
					seriesKey="req"
				/>
			</DashboardGraphCard>
			<DashboardGraphCard title="REQ query latency" info={DASHBOARD_CHART_INFO.latency}>
				<DashboardLatencyChart data={latencyChartData} />
			</DashboardGraphCard>
			<DashboardGraphCard title="Open subscriptions" info={DASHBOARD_CHART_INFO.subs}>
				<DashboardLineSeriesChart
					data={subsChartData}
					seriesLabel="Subs"
					color="var(--chart-4)"
					seriesKey="subs"
				/>
			</DashboardGraphCard>
		</div>
	{/if}
</div>
