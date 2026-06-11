<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { adminFetch } from '$lib/admin-api';
	import {
		ADMIN_CONN_REFRESH_ALLOWED,
		readAdminConnRefreshSecFromStorage,
		writeAdminConnRefreshSecToStorage,
		type AdminConnRefreshSec
	} from '$lib/admin-conn-poll-preference';
	import AdminPageHeading from '$lib/components/AdminPageHeading.svelte';
	import ConnectionReqEventDeltaChart from '$lib/components/ConnectionReqEventDeltaChart.svelte';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Label } from '$lib/components/ui/label';
	import * as Table from '$lib/components/ui/table';
	import * as Sheet from '$lib/components/ui/sheet';
	import Activity from '@lucide/svelte/icons/activity';
	import ChevronDown from '@lucide/svelte/icons/chevron-down';

	const CLOSED_PAGE_SIZES = [50, 100, 250, 500] as const;
	type ClosedPageSize = (typeof CLOSED_PAGE_SIZES)[number];

	function isClosedPageSize(n: number): n is ClosedPageSize {
		return (CLOSED_PAGE_SIZES as readonly number[]).includes(n);
	}

	type SeriesPoint = { t: number; auth: number; req: number; ev: number };

	type LiveRow = {
		ref: string;
		conn_id: string;
		peer_ip: string;
		remote_addr: string;
		started_unix: number;
		subscriptions: number;
		total_auth: number;
		total_req: number;
		total_client_event: number;
		series: unknown;
	};

	type ClosedRow = LiveRow & {
		id: number;
		ended_unix: number;
	};

	type RecentRow = LiveRow & {
		ended_unix: number;
	};

	type ConnListResp = {
		retention_days: number;
		live: LiveRow[];
		recent?: RecentRow[];
		closed: ClosedRow[];
		closed_total?: number;
	};

	type SubDetail = {
		sub_id: string;
		opened_unix: number;
		filter_count: number;
		kinds?: number[];
		initial_events_sent: number;
		initial_events_dropped: number;
		broadcast_events_enqueued: number;
		broadcast_events_dropped: number;
		eose_sent: number;
	};

	type ConnDetailResp = {
		kind: string;
		ref: string;
		conn_id: string;
		peer_ip: string;
		remote_addr: string;
		started_unix: number;
		ended_unix?: number | null;
		subscriptions: number;
		total_auth: number;
		total_req: number;
		total_client_event: number;
		series: unknown;
		subscription_details?: SubDetail[];
	};

	let retentionDays = $state(30);
	let liveRows = $state<LiveRow[]>([]);
	let recentRows = $state<RecentRow[]>([]);
	let listErr = $state<string | null>(null);
	let listLoading = $state(true);
	let liveBootstrapped = $state(false);

	let closedRows = $state<ClosedRow[]>([]);
	let closedTotal = $state<number | null>(null);
	let closedPage = $state(1);
	let closedPageSize = $state<ClosedPageSize>(50);
	let closedErr = $state<string | null>(null);
	let closedLoading = $state(false);
	let previousExpanded = $state(false);

	let selectedRef = $state<string | null>(null);
	let detailSheetOpen = $state(false);
	let detail = $state<ConnDetailResp | null>(null);
	let detailErr = $state<string | null>(null);
	let detailLoading = $state(false);

	let refreshSec = $state<AdminConnRefreshSec>(5);
	let listTimer: ReturnType<typeof setInterval> | null = null;
	let detailTimer: ReturnType<typeof setInterval> | null = null;

	const closedTotalPages = $derived(
		closedTotal === null ? 1 : Math.max(1, Math.ceil(closedTotal / closedPageSize))
	);

	const closedRangeLabel = $derived.by(() => {
		if (closedTotal === null || closedTotal === 0) {
			return '0 rows';
		}
		const from = (closedPage - 1) * closedPageSize + 1;
		const to = Math.min(closedPage * closedPageSize, closedTotal);
		return `Rows ${from}–${to} of ${closedTotal}`;
	});

	function parseSeries(raw: unknown): SeriesPoint[] {
		if (!Array.isArray(raw)) return [];
		const out: SeriesPoint[] = [];
		for (const x of raw) {
			if (!x || typeof x !== 'object') continue;
			const o = x as Record<string, unknown>;
			const t = Number(o.t);
			const auth = Number(o.auth ?? 0);
			const req = Number(o.req);
			const ev = Number(o.ev);
			if (!Number.isFinite(t) || !Number.isFinite(auth) || !Number.isFinite(req) || !Number.isFinite(ev))
				continue;
			out.push({ t, auth, req, ev });
		}
		return out;
	}

	let detailPts = $derived(detail ? parseSeries(detail.series) : []);

	async function loadLive() {
		listErr = null;
		if (!liveBootstrapped) {
			listLoading = true;
		}
		try {
			const res = await adminFetch('/api/audit/connections?include_closed=0');
			if (!res.ok) {
				listErr = `HTTP ${res.status}`;
				return;
			}
			const j = (await res.json()) as ConnListResp;
			retentionDays = typeof j.retention_days === 'number' ? j.retention_days : 30;
			liveRows = Array.isArray(j.live) ? j.live : [];
			recentRows = Array.isArray(j.recent) ? j.recent : [];
		} catch (e) {
			listErr = e instanceof Error ? e.message : 'load failed';
		} finally {
			listLoading = false;
			liveBootstrapped = true;
		}
	}

	async function loadClosedPaged(allowClampRetry = true) {
		if (!previousExpanded) return;
		closedErr = null;
		closedLoading = true;
		try {
			const q = new URLSearchParams();
			q.set('include_live', '0');
			q.set('include_closed', '1');
			q.set('limit', String(closedPageSize));
			q.set('offset', String((closedPage - 1) * closedPageSize));
			const res = await adminFetch(`/api/audit/connections?${q}`);
			if (!res.ok) {
				closedErr = `HTTP ${res.status}`;
				return;
			}
			const j = (await res.json()) as ConnListResp;
			const total =
				typeof j.closed_total === 'number' && Number.isFinite(j.closed_total) ? j.closed_total : 0;
			closedTotal = total;
			closedRows = Array.isArray(j.closed) ? j.closed : [];

			const tp = Math.max(1, Math.ceil(total / closedPageSize));
			if (closedPage > tp && tp >= 1) {
				closedPage = tp;
				if (allowClampRetry) {
					await loadClosedPaged(false);
				}
				return;
			}
		} catch (e) {
			closedErr = e instanceof Error ? e.message : 'load failed';
		} finally {
			closedLoading = false;
		}
	}

	async function reloadVisible() {
		await loadLive();
		if (previousExpanded) await loadClosedPaged();
	}

	async function loadDetailRef(ref: string) {
		detailErr = null;
		detailLoading = true;
		detail = null;
		try {
			const enc = encodeURIComponent(ref);
			const res = await adminFetch(`/api/audit/connections/${enc}`);
			if (!res.ok) {
				detailErr = `HTTP ${res.status}`;
				return;
			}
			detail = (await res.json()) as ConnDetailResp;
		} catch (e) {
			detailErr = e instanceof Error ? e.message : 'load failed';
		} finally {
			detailLoading = false;
		}
	}

	function startListPoll() {
		if (listTimer) {
			clearInterval(listTimer);
			listTimer = null;
		}
		if (refreshSec === 0) return;
		listTimer = setInterval(() => {
			if (!selectedRef) void loadLive();
			if (previousExpanded) void loadClosedPaged();
		}, refreshSec * 1000);
	}

	function selectRow(ref: string) {
		selectedRef = ref;
		detailSheetOpen = true;
		void loadDetailRef(ref);
	}

	function closeDetail() {
		detailSheetOpen = false;
		selectedRef = null;
		detail = null;
		detailErr = null;
		if (detailTimer) {
			clearInterval(detailTimer);
			detailTimer = null;
		}
		void reloadVisible();
	}

	function fmtDuration(started: number, ended?: number) {
		const end = ended ?? Math.floor(Date.now() / 1000);
		const sec = Math.max(0, end - started);
		if (sec < 60) return `${sec}s`;
		const m = Math.floor(sec / 60);
		if (m < 60) return `${m}m`;
		const h = Math.floor(m / 60);
		return `${h}h${m % 60}m`;
	}

	/** Human-readable time since `endedUnix` (disconnect). */
	function fmtClosedAgo(endedUnix: number): string {
		const now = Math.floor(Date.now() / 1000);
		const sec = Math.max(0, now - endedUnix);
		if (sec < 60) return `${sec}s ago`;
		const m = Math.floor(sec / 60);
		if (m < 60) return `${m}m ago`;
		const h = Math.floor(m / 60);
		if (h < 24) return `${h}h ago`;
		const d = Math.floor(h / 24);
		return `${d}d ago`;
	}

	function closedGoPrev() {
		if (closedPage <= 1) return;
		closedPage -= 1;
		void loadClosedPaged();
	}

	function closedGoNext() {
		if (closedPage >= closedTotalPages) return;
		closedPage += 1;
		void loadClosedPaged();
	}

	function onClosedPageSizeChange(ev: Event) {
		const v = Number.parseInt((ev.currentTarget as HTMLSelectElement).value, 10);
		closedPageSize = isClosedPageSize(v) ? v : 50;
		closedPage = 1;
		void loadClosedPaged();
	}

	$effect(() => {
		writeAdminConnRefreshSecToStorage(refreshSec);
		if (listTimer) {
			clearInterval(listTimer);
			listTimer = null;
		}
		if (detailTimer) {
			clearInterval(detailTimer);
			detailTimer = null;
		}
		startListPoll();
		if (selectedRef && refreshSec > 0) {
			const ref = selectedRef;
			detailTimer = setInterval(() => {
				if (selectedRef === ref) void loadDetailRef(ref);
			}, refreshSec * 1000);
		}
	});

	onMount(() => {
		refreshSec = readAdminConnRefreshSecFromStorage();
		void loadLive();
	});

	onDestroy(() => {
		if (listTimer) clearInterval(listTimer);
		if (detailTimer) clearInterval(detailTimer);
	});

	const selectClass =
		'border-input dark:bg-input/30 focus-visible:border-ring focus-visible:ring-ring/50 h-8 rounded-lg border bg-transparent px-2.5 text-sm shadow-xs outline-none focus-visible:ring-3';

	const selectClassNarrow = selectClass + ' max-w-[140px]';
</script>

<div class="space-y-6">
	<AdminPageHeading
		title="Audit · Connections"
		subtitle="Live and recent WebSocket sessions. Retention matches audit log ({retentionDays} days)."
		Icon={Activity}
	/>

	<div class="flex flex-wrap items-end gap-4">
		<div class="space-y-1.5">
			<Label for="conn-refresh">Auto-refresh</Label>
			<select
				id="conn-refresh"
				class={selectClass}
				value={String(refreshSec)}
				aria-label="Polling interval for this page"
				onchange={(e) => {
					const n = Number.parseInt(e.currentTarget.value, 10);
					if ((ADMIN_CONN_REFRESH_ALLOWED as readonly number[]).includes(n)) {
						refreshSec = n as AdminConnRefreshSec;
					}
				}}
			>
				{#each ADMIN_CONN_REFRESH_ALLOWED as s (s)}
					<option value={String(s)}>{s === 0 ? 'Off' : `${s}s`}</option>
				{/each}
			</select>
		</div>
		<Button variant="outline" size="sm" type="button" onclick={() => void reloadVisible()}>Reload</Button>
	</div>

	{#if listErr}
		<p class="text-sm text-destructive">{listErr}</p>
	{/if}

	<Card.Root>
		<Card.Header class="pb-2">
			<Card.Title class="text-base">Live</Card.Title>
			<Card.Description>Open WebSocket connections in this relay process.</Card.Description>
		</Card.Header>
		<Card.Content class="pt-0">
			{#if listLoading}
				<p class="text-muted-foreground py-6 text-sm">Loading…</p>
			{:else if liveRows.length === 0}
				<p class="text-muted-foreground py-6 text-sm">No open connections.</p>
			{:else}
				<div class="overflow-x-auto">
					<Table.Root>
						<Table.Header>
							<Table.Row>
								<Table.Head>Conn</Table.Head>
								<Table.Head>IP</Table.Head>
								<Table.Head class="text-right">Subs</Table.Head>
								<Table.Head>Duration</Table.Head>
								<Table.Head class="w-[8.5rem] min-w-[8.5rem] max-w-[10rem]">AUTH / REQ / EVENT</Table.Head>
							</Table.Row>
						</Table.Header>
						<Table.Body>
							{#each liveRows as row (row.ref)}
								{@const pts = parseSeries(row.series)}
								<Table.Row
									class={selectedRef === row.ref ? 'bg-muted/50' : 'cursor-pointer hover:bg-muted/30'}
									onclick={() => selectRow(row.ref)}
								>
									<Table.Cell class="font-mono text-xs">{row.conn_id}</Table.Cell>
									<Table.Cell class="text-xs">{row.peer_ip}</Table.Cell>
									<Table.Cell class="text-right tabular-nums">{row.subscriptions}</Table.Cell>
									<Table.Cell class="text-xs text-muted-foreground"
										>{fmtDuration(row.started_unix)}</Table.Cell
									>
									<Table.Cell class="py-0.5 align-middle">
										<ConnectionReqEventDeltaChart {pts} compact />
									</Table.Cell>
								</Table.Row>
							{/each}
						</Table.Body>
					</Table.Root>
				</div>
			{/if}
		</Card.Content>
	</Card.Root>

	<Card.Root>
		<Card.Header class="pb-2">
			<Card.Title class="text-base">Recent</Card.Title>
			<Card.Description
				>Up to 1000 recently closed connections kept in relay memory (this process).</Card.Description
			>
		</Card.Header>
		<Card.Content class="pt-0">
			{#if listLoading}
				<p class="text-muted-foreground py-6 text-sm">Loading…</p>
			{:else if recentRows.length === 0}
				<p class="text-muted-foreground py-6 text-sm">No recent disconnects in memory.</p>
			{:else}
				<div class="overflow-x-auto">
					<Table.Root>
						<Table.Header>
							<Table.Row>
								<Table.Head>Conn</Table.Head>
								<Table.Head>IP</Table.Head>
								<Table.Head class="text-right">Subs</Table.Head>
								<Table.Head>Duration</Table.Head>
								<Table.Head class="whitespace-nowrap">Closed</Table.Head>
								<Table.Head class="w-[8.5rem] min-w-[8.5rem] max-w-[10rem]">AUTH / REQ / EVENT</Table.Head>
							</Table.Row>
						</Table.Header>
						<Table.Body>
							{#each recentRows as row (row.ref)}
								{@const pts = parseSeries(row.series)}
								<Table.Row
									class={selectedRef === row.ref ? 'bg-muted/50' : 'cursor-pointer hover:bg-muted/30'}
									onclick={() => selectRow(row.ref)}
								>
									<Table.Cell class="font-mono text-xs">{row.conn_id}</Table.Cell>
									<Table.Cell class="text-xs">{row.peer_ip}</Table.Cell>
									<Table.Cell class="text-right tabular-nums">{row.subscriptions}</Table.Cell>
									<Table.Cell class="text-xs text-muted-foreground"
										>{fmtDuration(row.started_unix, row.ended_unix)}</Table.Cell
									>
									<Table.Cell
										class="text-xs tabular-nums text-muted-foreground whitespace-nowrap"
										title={new Date(row.ended_unix * 1000).toISOString()}
									>
										{fmtClosedAgo(row.ended_unix)}
									</Table.Cell>
									<Table.Cell class="py-0.5 align-middle">
										<ConnectionReqEventDeltaChart {pts} compact />
									</Table.Cell>
								</Table.Row>
							{/each}
						</Table.Body>
					</Table.Root>
				</div>
			{/if}
		</Card.Content>
	</Card.Root>

	<Card.Root>
		<Card.Header class="pb-2">
			<Card.Title class="text-base">Previous</Card.Title>
			<Card.Description
				>Closed sessions (paginated). Expand to load from the database; collapsed sections are not
				refreshed automatically.</Card.Description
			>
		</Card.Header>
		<details
			class="group border-t border-border"
			bind:open={previousExpanded}
			ontoggle={(e) => {
				if (e.currentTarget.open) void loadClosedPaged();
			}}
		>
			<summary
				class="flex cursor-pointer list-none items-center gap-2 px-6 py-3 text-sm font-medium text-foreground outline-none marker:content-none hover:bg-muted/30 [&::-webkit-details-marker]:hidden"
			>
				<ChevronDown
					class="text-muted-foreground size-4 shrink-0 transition-transform group-open:rotate-180"
					aria-hidden="true"
				/>
				<span>Show closed sessions</span>
				{#if previousExpanded && closedTotal !== null && closedTotal > 0}
					<span class="text-muted-foreground font-normal tabular-nums">({closedTotal} total)</span>
				{/if}
			</summary>
			<div class="space-y-0 px-6 pb-6">
				{#if closedLoading && closedRows.length === 0 && closedTotal === null}
					<p class="text-muted-foreground py-4 text-sm">Loading…</p>
				{:else if closedErr}
					<p class="text-sm text-destructive">{closedErr}</p>
				{:else if closedTotal === 0}
					<p class="text-muted-foreground py-4 text-sm">No closed sessions in retention window.</p>
				{:else}
					<div class="overflow-x-auto">
						<Table.Root>
							<Table.Header>
								<Table.Row>
									<Table.Head>Conn</Table.Head>
									<Table.Head>IP</Table.Head>
									<Table.Head class="text-right">Subs*</Table.Head>
									<Table.Head>Duration</Table.Head>
									<Table.Head class="whitespace-nowrap" title="Time since the connection closed"
										>Closed</Table.Head
									>
									<Table.Head class="w-[8.5rem] min-w-[8.5rem] max-w-[10rem]">AUTH / REQ / EVENT</Table.Head>
								</Table.Row>
							</Table.Header>
							<Table.Body>
								{#each closedRows as row (row.ref)}
									{@const pts = parseSeries(row.series)}
									<Table.Row
										class={selectedRef === row.ref ? 'bg-muted/50' : 'cursor-pointer hover:bg-muted/30'}
										onclick={() => selectRow(row.ref)}
									>
										<Table.Cell class="font-mono text-xs">{row.conn_id}</Table.Cell>
										<Table.Cell class="text-xs">{row.peer_ip}</Table.Cell>
										<Table.Cell class="text-right text-xs text-muted-foreground">—</Table.Cell>
										<Table.Cell class="text-xs text-muted-foreground"
											>{fmtDuration(row.started_unix, row.ended_unix)}</Table.Cell
										>
										<Table.Cell
											class="text-xs tabular-nums text-muted-foreground whitespace-nowrap"
											title={new Date(row.ended_unix * 1000).toISOString()}
										>
											{fmtClosedAgo(row.ended_unix)}
										</Table.Cell>
										<Table.Cell class="py-0.5 align-middle">
											<ConnectionReqEventDeltaChart {pts} compact />
										</Table.Cell>
									</Table.Row>
								{/each}
							</Table.Body>
						</Table.Root>
					</div>
					<div
						class="mt-3 flex flex-col gap-3 border-t border-border pt-3 sm:flex-row sm:flex-wrap sm:items-center sm:justify-between"
					>
						<p class="text-sm text-muted-foreground">{closedRangeLabel}</p>
						<div class="flex flex-wrap items-center gap-4">
							<div class="flex items-center gap-2">
								<Label for="closed-page-size" class="text-muted-foreground shrink-0 text-sm font-normal"
									>Per page</Label
								>
								<select
									id="closed-page-size"
									class={selectClassNarrow}
									value={String(closedPageSize)}
									onchange={onClosedPageSizeChange}
									aria-label="Closed sessions per page"
								>
									{#each CLOSED_PAGE_SIZES as sz (sz)}
										<option value={String(sz)}>{sz}</option>
									{/each}
								</select>
							</div>
							<div class="flex flex-wrap items-center gap-2">
								<Button type="button" variant="outline" size="sm" disabled={closedPage <= 1} onclick={closedGoPrev}>
									Previous
								</Button>
								<span class="text-sm tabular-nums text-muted-foreground">
									Page {closedPage} / {closedTotalPages}
								</span>
								<Button
									type="button"
									variant="outline"
									size="sm"
									disabled={closedPage >= closedTotalPages}
									onclick={closedGoNext}
								>
									Next
								</Button>
							</div>
						</div>
					</div>
				{/if}
				<p class="text-muted-foreground mt-3 text-xs">
					*Subscription counts for closed rows are shown in the detail panel from the disconnect snapshot.
				</p>
			</div>
		</details>
	</Card.Root>

	<Sheet.Root bind:open={detailSheetOpen} onOpenChange={(o) => { if (!o) closeDetail(); }}>
		<Sheet.Content class="flex w-[min(100vw-1rem,32rem)] max-w-full flex-col gap-0 sm:max-w-lg">
			<Sheet.Header class="border-b border-border px-4 py-4 text-left">
				<Sheet.Title class="text-base">Connection detail</Sheet.Title>
				<Sheet.Description class="font-mono text-xs break-all">
					{selectedRef ?? ''}
				</Sheet.Description>
			</Sheet.Header>
			<div class="flex-1 overflow-y-auto px-4 py-4">
				{#if detailLoading}
					<p class="text-muted-foreground text-sm">Loading detail…</p>
				{:else if detailErr}
					<p class="text-sm text-destructive">{detailErr}</p>
				{:else if detail}
					<dl class="grid grid-cols-2 gap-x-3 gap-y-2 text-sm">
						<dt class="text-muted-foreground">Kind</dt>
						<dd>{detail.kind}</dd>
						<dt class="text-muted-foreground">Conn</dt>
						<dd class="font-mono text-xs">{detail.conn_id}</dd>
						<dt class="text-muted-foreground">IP</dt>
						<dd class="text-xs">{detail.peer_ip}</dd>
						<dt class="text-muted-foreground">Subscriptions</dt>
						<dd class="tabular-nums">{detail.subscriptions}</dd>
						<dt class="text-muted-foreground">AUTH</dt>
						<dd class="tabular-nums">{detail.total_auth ?? 0}</dd>
						<dt class="text-muted-foreground">REQ</dt>
						<dd class="tabular-nums">{detail.total_req}</dd>
						<dt class="text-muted-foreground">EVENT</dt>
						<dd class="tabular-nums">{detail.total_client_event}</dd>
					</dl>
					<div class="mt-4 space-y-2">
						<p class="text-sm font-medium">Series (per bucket Δ)</p>
						<ConnectionReqEventDeltaChart pts={detailPts} />
					</div>
					<div class="mt-6">
						<p class="text-sm font-medium">Subscriptions</p>
						{#if !detail.subscription_details || detail.subscription_details.length === 0}
							<p class="text-muted-foreground mt-2 text-sm">None recorded.</p>
						{:else}
							<ul class="mt-2 space-y-3">
								{#each detail.subscription_details as s (s.sub_id)}
									<li class="rounded-md border border-border p-3 text-sm">
										<p class="font-mono text-xs font-medium">{s.sub_id}</p>
										<p class="text-muted-foreground mt-1 text-xs">
											Filters: {s.filter_count}
											{#if s.kinds && s.kinds.length}
												· kinds {s.kinds.slice(0, 8).join(', ')}{s.kinds.length > 8 ? '…' : ''}
											{/if}
										</p>
										<p class="mt-2 text-xs tabular-nums">
											initial sent/drop {s.initial_events_sent}/{s.initial_events_dropped} · broadcast ok/drop
											{s.broadcast_events_enqueued}/{s.broadcast_events_dropped} · EOSE {s.eose_sent}
										</p>
									</li>
								{/each}
							</ul>
						{/if}
					</div>
				{/if}
			</div>
		</Sheet.Content>
	</Sheet.Root>
</div>
