<script lang="ts">
	import { onMount } from 'svelte';
	import LayoutDashboard from '@lucide/svelte/icons/layout-dashboard';
	import { adminFetch } from '$lib/admin-api';
	import { formatBytes } from '$lib/format-bytes';
	import { formatDurationSec } from '$lib/format-duration';
	import AdminPageHeading from '$lib/components/AdminPageHeading.svelte';
	import * as Card from '$lib/components/ui/card';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Switch } from '$lib/components/ui/switch';
	import { Label } from '$lib/components/ui/label';

	type Bucket = {
		bucket_start_unix: number;
		events_stored: number;
		events_rejected: number;
		req_count: number;
		close_count: number;
		query_ms_avg?: number;
	};

	type Stats = {
		open_connections?: number;
		relay_port?: number;
		admin_port?: number;
		relay_version?: string;
		subscriptions_open?: number;
		started_at_unix?: number;
		uptime_sec?: number;
		relay_counters?: Record<string, number>;
		recent_query_ms?: number[];
		storage?: { bytes?: number; events?: number; tags?: number; audit?: number };
		series?: { bucket_sec?: number; buckets?: Bucket[] };
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

	const pollMs = 4000;

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

	onMount(async () => {
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
		if (typeof window === 'undefined') return;
		const id = window.setInterval(() => {
			void refreshStats();
		}, pollMs);
		return () => window.clearInterval(id);
	});

	function bucketTimeLabel(u: number): string {
		return new Date(u * 1000).toLocaleString(undefined, {
			month: 'short',
			day: 'numeric',
			hour: '2-digit',
			minute: '2-digit'
		});
	}

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

	function trafficChartPoints(buckets: Bucket[]): { ev: string; rq: string } {
		if (!buckets.length) return { ev: '', rq: '' };
		const maxE = Math.max(1, ...buckets.map((b) => b.events_stored));
		const maxR = Math.max(1, ...buckets.map((b) => b.req_count));
		const w = 400;
		const h = 120;
		const pad = 8;
		const n = buckets.length;
		const evPts: string[] = [];
		const rqPts: string[] = [];
		for (let i = 0; i < n; i++) {
			const x = pad + (i * (w - 2 * pad)) / Math.max(1, n - 1);
			const evY = h - pad - (buckets[i].events_stored / maxE) * (h - 2 * pad);
			const rqY = h - pad - (buckets[i].req_count / maxR) * (h - 2 * pad);
			evPts.push(`${x},${evY}`);
			rqPts.push(`${x},${rqY}`);
		}
		return { ev: evPts.join(' '), rq: rqPts.join(' ') };
	}

	function latencyChartPoints(ms: number[]): string {
		if (!ms.length) return '';
		const maxV = Math.max(1, ...ms);
		const w = 400;
		const h = 100;
		const pad = 8;
		const n = ms.length;
		const pts: string[] = [];
		for (let i = 0; i < n; i++) {
			const x = pad + (i * (w - 2 * pad)) / Math.max(1, n - 1);
			const y = h - pad - (ms[i] / maxV) * (h - 2 * pad);
			pts.push(`${x},${y}`);
		}
		return pts.join(' ');
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
				<span>Refreshing every {pollMs / 1000}s</span>
			{/if}
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
			<Card.Root>
				<Card.Header class="pb-2">
					<Card.Description>WebSocket connections</Card.Description>
					<Card.Title class="text-3xl tabular-nums">{stats.open_connections ?? 0}</Card.Title>
				</Card.Header>
				<Card.Content>
					<Badge variant="secondary">live</Badge>
				</Card.Content>
			</Card.Root>
			<Card.Root>
				<Card.Header class="pb-2">
					<Card.Description>Subscriptions</Card.Description>
					<Card.Title class="text-3xl tabular-nums">{stats.subscriptions_open ?? 0}</Card.Title>
				</Card.Header>
			</Card.Root>
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
			<Card.Root>
				<Card.Header class="pb-2">
					<Card.Description>Events stored (ok)</Card.Description>
					<Card.Title class="text-3xl tabular-nums">{counter('events_stored_ok')}</Card.Title>
				</Card.Header>
			</Card.Root>
			<Card.Root>
				<Card.Header class="pb-2">
					<Card.Description>Events rejected</Card.Description>
					<Card.Title class="text-3xl tabular-nums">{counter('events_rejected')}</Card.Title>
				</Card.Header>
			</Card.Root>
			<Card.Root>
				<Card.Header class="pb-2">
					<Card.Description>REQ / CLOSE (total)</Card.Description>
					<Card.Title class="text-3xl tabular-nums">
						{counter('req_total')} / {counter('close_total')}
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
				<div>Messages / IP: <span class="font-mono">{counter('rate_limit_messages')}</span></div>
				<div>Bandwidth: <span class="font-mono">{counter('rate_limit_bandwidth')}</span></div>
				<div>Events / conn: <span class="font-mono">{counter('rate_limit_events')}</span></div>
				<div>REQ / conn: <span class="font-mono">{counter('rate_limit_reqs')}</span></div>
				<div>New conn / IP: <span class="font-mono">{counter('rate_limit_new_connections')}</span></div>
				<div>Max connections: <span class="font-mono">{counter('rate_limit_max_connections')}</span></div>
			</Card.Content>
		</Card.Root>

		<div class="grid gap-4 lg:grid-cols-2">
			<Card.Root>
				<Card.Header class="pb-2">
					<Card.Title class="text-base">Traffic (per minute)</Card.Title>
					<Card.Description>Events stored vs REQ count (UTC buckets)</Card.Description>
				</Card.Header>
				<Card.Content>
					{#if stats.series?.buckets?.length}
						{@const pts = trafficChartPoints(stats.series.buckets)}
						<svg
							class="text-chart-1 h-40 w-full max-w-xl"
							viewBox="0 0 400 120"
							preserveAspectRatio="none"
							role="img"
							aria-label="Traffic chart"
						>
							<rect width="400" height="120" class="fill-muted/30" rx="4" />
							<polyline
								fill="none"
								stroke="var(--chart-1)"
								stroke-width="2"
								points={pts.ev}
								vector-effect="non-scaling-stroke"
							/>
							<polyline
								fill="none"
								stroke="var(--chart-2)"
								stroke-width="2"
								points={pts.rq}
								vector-effect="non-scaling-stroke"
							/>
						</svg>
						<div class="text-muted-foreground mt-2 flex justify-between text-xs">
							<span>{bucketTimeLabel(stats.series.buckets[0].bucket_start_unix)}</span>
							<span
								>{bucketTimeLabel(
									stats.series.buckets[stats.series.buckets.length - 1].bucket_start_unix
								)}</span
							>
						</div>
						<div class="text-muted-foreground mt-1 flex gap-4 text-xs">
							<span class="inline-flex items-center gap-1"
								><span class="inline-block h-2 w-4 rounded-sm" style="background: var(--chart-1)"></span> Events
								stored</span
							>
							<span class="inline-flex items-center gap-1"
								><span class="inline-block h-2 w-4 rounded-sm" style="background: var(--chart-2)"></span> REQ</span
							>
						</div>
					{:else}
						<p class="text-muted-foreground text-sm">No bucket data yet (wait up to one minute).</p>
					{/if}
				</Card.Content>
			</Card.Root>

			<Card.Root>
				<Card.Header class="pb-2">
					<Card.Title class="text-base">REQ query latency</Card.Title>
					<Card.Description>Recent samples (ms) + per-minute average in table above</Card.Description>
				</Card.Header>
				<Card.Content>
					{#if stats.recent_query_ms?.length}
						{@const lp = latencyChartPoints(stats.recent_query_ms)}
						<svg
							class="h-36 w-full max-w-xl"
							viewBox="0 0 400 100"
							preserveAspectRatio="none"
							role="img"
							aria-label="Latency chart"
						>
							<rect width="400" height="100" class="fill-muted/30" rx="4" />
							<polyline
								fill="none"
								stroke="var(--chart-3)"
								stroke-width="2"
								points={lp}
								vector-effect="non-scaling-stroke"
							/>
						</svg>
					{:else}
						<p class="text-muted-foreground text-sm">No latency samples yet.</p>
					{/if}
				</Card.Content>
			</Card.Root>
		</div>

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
