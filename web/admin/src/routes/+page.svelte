<script lang="ts">
	import { onMount } from 'svelte';
	import { browser } from '$app/environment';
	import LayoutDashboard from '@lucide/svelte/icons/layout-dashboard';
	import { adminFetch } from '$lib/admin-api';
	import { formatBytes } from '$lib/format-bytes';
	import { formatDurationSec } from '$lib/format-duration';
	import { formatCompactCount } from '$lib/format-compact-count';
	import {
		LS_REFRESH_SEC,
		LS_RESOLUTION,
		LS_TIME_RANGE,
		TIME_RANGE_MS,
		binLatencySamples,
		bucketSeries,
		filterBucketsByRange,
		filterLatencyByRange,
		type LatencySample,
		type MetricBucket,
		type Resolution,
		type TimeRangePreset
	} from '$lib/dashboard-metrics';
	import AdminPageHeading from '$lib/components/AdminPageHeading.svelte';
	import AnalyticsStatCard from '$lib/components/AnalyticsStatCard.svelte';
	import DashboardGraphCard from '$lib/components/DashboardGraphCard.svelte';
	import DashboardLineSeriesChart from '$lib/components/DashboardLineSeriesChart.svelte';
	import * as Card from '$lib/components/ui/card';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Switch } from '$lib/components/ui/switch';
	import { Label } from '$lib/components/ui/label';

	const selectClass =
		'border-input bg-background focus-visible:ring-ring/50 h-9 min-w-[8rem] rounded-md border px-2.5 text-sm shadow-xs focus-visible:ring-2 focus-visible:outline-none';

	type Stats = {
		open_connections?: number;
		relay_port?: number;
		admin_port?: number;
		relay_version?: string;
		subscriptions_open?: number;
		started_at_unix?: number;
		uptime_sec?: number;
		relay_counters?: Record<string, number>;
		recent_query_latency?: LatencySample[];
		storage?: { bytes?: number; events?: number; tags?: number; audit?: number };
		series?: { bucket_sec?: number; buckets?: MetricBucket[] };
	};

	type RelayIdentity = {
		pubkey_hex: string;
		npub: string;
	};

	let stats = $state<Stats | null>(null);
	let relayIdentity = $state.raw<RelayIdentity | null>(null);
	let loadErr = $state<string | null>(null);
	let relayIdErr = $state<string | null>(null);
	let loading = $state(true);
	let pollErr = $state<string | null>(null);
	let lastUpdated = $state<number | null>(null);
	let showNpub = $state(true);
	let copyHint = $state<string | null>(null);

	let timeRange = $state<TimeRangePreset>('1h');
	let resolution = $state<Resolution>('minute');
	let refreshSec = $state(5);

	onMount(async () => {
		const tr = localStorage.getItem(LS_TIME_RANGE);
		if (tr === '15m' || tr === '1h' || tr === '6h' || tr === '24h') timeRange = tr;
		const res = localStorage.getItem(LS_RESOLUTION);
		if (res === 'minute' || res === 'second' || res === 'hour') resolution = res;
		const rs = localStorage.getItem(LS_REFRESH_SEC);
		const rv = rs ? Number(rs) : NaN;
		if ([3, 5, 10, 30, 60].includes(rv)) refreshSec = rv;

		try {
			const [statsRes, idRes] = await Promise.all([
				adminFetch('/api/stats'),
				adminFetch('/api/relay-identity')
			]);
			if (!statsRes.ok) {
				loadErr = statsRes.status === 401 ? 'Unauthorized' : `HTTP ${statsRes.status}`;
				return;
			}
			stats = (await statsRes.json()) as Stats;
			lastUpdated = Date.now();
			if (idRes.ok) {
				relayIdentity = (await idRes.json()) as RelayIdentity;
			} else if (idRes.status === 503) {
				relayIdErr = 'Relay identity is not available on this process.';
			} else {
				relayIdErr = idRes.status === 401 ? 'Unauthorized' : `HTTP ${idRes.status}`;
			}
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

	async function refreshStats() {
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
		}
	}

	$effect(() => {
		if (!browser || loading) return;
		const ms = refreshSec * 1000;
		const id = window.setInterval(() => {
			void refreshStats();
		}, ms);
		return () => window.clearInterval(id);
	});

	function visiblePubkey(): string {
		if (!relayIdentity) return '';
		return showNpub ? relayIdentity.npub : relayIdentity.pubkey_hex;
	}

	async function copyVisiblePubkey() {
		const t = visiblePubkey();
		if (!t) return;
		try {
			await navigator.clipboard.writeText(t);
			copyHint = 'Copied';
			window.setTimeout(() => (copyHint = null), 2000);
		} catch {
			copyHint = 'Copy failed';
			window.setTimeout(() => (copyHint = null), 2000);
		}
	}

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
		const samples = stats?.recent_query_latency ?? [];
		const f = filterLatencyByRange(samples, rangeMs);
		return binLatencySamples(f, resolution);
	});

	function graphSubtitle(metric: 'counter' | 'subs'): string {
		if (metric === 'subs') {
			if (resolution === 'hour') return 'Last open-count sample in each UTC hour.';
			if (resolution === 'second') return 'Snapshot per minute (same points as per-minute view).';
			return 'Open REQ subscriptions sampled each UTC minute.';
		}
		if (resolution === 'minute') return 'Counts per UTC minute bucket.';
		if (resolution === 'second') return 'Average per second within each minute (count ÷ 60).';
		return 'Totals summed into UTC hour bins.';
	}

	function latencySubtitle(): string {
		if (resolution === 'minute') return 'Mean latency per UTC minute (from timestamped samples).';
		if (resolution === 'second') return 'Mean latency per UTC second.';
		return 'Mean latency per UTC hour.';
	}
</script>

<div class="space-y-6">
	<AdminPageHeading
		title="Dashboard"
		subtitle="Live relay health, traffic, and storage."
		Icon={LayoutDashboard}
	/>

	{#if loading}
		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
			{#each Array(4) as _, i (i)}
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

		<div
			class="border-border bg-muted/30 flex flex-wrap items-end gap-4 rounded-lg border p-4"
			aria-label="Chart range and refresh options"
		>
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
					<option value="second">Per second (rates)</option>
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

		<div class="grid gap-4 lg:grid-cols-2">
			<DashboardGraphCard title="Events stored" description={graphSubtitle('counter')}>
				<DashboardLineSeriesChart
					data={eventsChartData}
					seriesLabel="Events"
					color="var(--chart-1)"
					seriesKey="events"
				/>
			</DashboardGraphCard>
			<DashboardGraphCard title="REQ count" description={graphSubtitle('counter')}>
				<DashboardLineSeriesChart
					data={reqChartData}
					seriesLabel="REQ"
					color="var(--chart-2)"
					seriesKey="req"
				/>
			</DashboardGraphCard>
			<DashboardGraphCard title="REQ query latency" description={latencySubtitle()}>
				<DashboardLineSeriesChart
					data={latencyChartData}
					seriesLabel="ms"
					color="var(--chart-3)"
					seriesKey="latency"
				/>
			</DashboardGraphCard>
			<DashboardGraphCard title="Open subscriptions" description={graphSubtitle('subs')}>
				<DashboardLineSeriesChart
					data={subsChartData}
					seriesLabel="Subs"
					color="var(--chart-4)"
					seriesKey="subs"
				/>
			</DashboardGraphCard>
		</div>

		{#if relayIdentity}
			<Card.Root class="sm:col-span-2 lg:col-span-3">
				<Card.Header class="pb-2">
					<Card.Description>Relay identity (read-only)</Card.Description>
					<div class="flex flex-wrap items-center gap-4">
						<Label for="npub-switch" class="text-sm font-medium">Show as npub</Label>
						<Switch id="npub-switch" bind:checked={showNpub} />
					</div>
				</Card.Header>
				<Card.Content class="space-y-3">
					<div class="flex flex-wrap items-start gap-2">
						<p class="font-mono text-sm leading-relaxed break-all">{visiblePubkey()}</p>
						<Button type="button" variant="outline" size="sm" onclick={() => void copyVisiblePubkey()}>
							Copy
						</Button>
						{#if copyHint}
							<span class="text-muted-foreground text-xs">{copyHint}</span>
						{/if}
					</div>
				</Card.Content>
			</Card.Root>
		{:else if relayIdErr}
			<Card.Root>
				<Card.Header class="pb-2">
					<Card.Description>Relay identity</Card.Description>
				</Card.Header>
				<Card.Content>
					<p class="text-destructive text-sm">{relayIdErr}</p>
				</Card.Content>
			</Card.Root>
		{/if}

		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
			<AnalyticsStatCard label="WebSocket connections" value={stats.open_connections ?? 0}>
				{#snippet badge()}
					<Badge variant="secondary">live</Badge>
				{/snippet}
			</AnalyticsStatCard>
			<AnalyticsStatCard label="Subscriptions" value={stats.subscriptions_open ?? 0} />
			<Card.Root>
				<Card.Header class="pb-2">
					<Card.Description>Uptime</Card.Description>
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
					<Card.Description>Storage (DB)</Card.Description>
					<Card.Title class="text-xl tabular-nums leading-snug">
						{formatBytes(stats.storage?.bytes ?? 0)}
					</Card.Title>
				</Card.Header>
				<Card.Content class="text-muted-foreground text-xs">
					{stats.storage?.events ?? 0} events · {stats.storage?.tags ?? 0} tags · {stats.storage?.audit ?? 0}
					audit rows
				</Card.Content>
			</Card.Root>
		</div>

		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
			<AnalyticsStatCard label="Events stored (ok)" value={counter('events_stored_ok')} compactNumber />
			<AnalyticsStatCard label="Events rejected" value={counter('events_rejected')} compactNumber />
			<Card.Root>
				<Card.Header class="pb-2">
					<Card.Description>REQ / CLOSE (total)</Card.Description>
					<Card.Title class="text-3xl tabular-nums">
						{formatCompactCount(counter('req_total'))} / {formatCompactCount(counter('close_total'))}
					</Card.Title>
				</Card.Header>
			</Card.Root>
			<Card.Root>
				<Card.Header class="pb-2">
					<Card.Description>Relay version</Card.Description>
					<Card.Title class="font-mono text-sm leading-relaxed break-all">{stats.relay_version ?? '—'}</Card.Title>
				</Card.Header>
			</Card.Root>
		</div>

		<Card.Root>
			<Card.Header class="pb-2">
				<Card.Title class="text-base">Rate limits (cumulative)</Card.Title>
				<Card.Description>Hits since relay process start</Card.Description>
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
			</Card.Content>
		</Card.Root>

		<div class="grid gap-4 sm:grid-cols-2">
			<Card.Root>
				<Card.Header class="pb-2">
					<Card.Description>Relay port</Card.Description>
					<Card.Title class="text-3xl tabular-nums">{stats.relay_port ?? '—'}</Card.Title>
				</Card.Header>
			</Card.Root>
			<Card.Root>
				<Card.Header class="pb-2">
					<Card.Description>Admin port</Card.Description>
					<Card.Title class="text-3xl tabular-nums">{stats.admin_port ?? '—'}</Card.Title>
				</Card.Header>
			</Card.Root>
		</div>
	{/if}
</div>
