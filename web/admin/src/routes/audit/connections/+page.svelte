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

	type SeriesPoint = { t: number; req: number; ev: number };

	type LiveRow = {
		ref: string;
		conn_id: string;
		peer_ip: string;
		remote_addr: string;
		started_unix: number;
		subscriptions: number;
		total_req: number;
		total_client_event: number;
		series: unknown;
	};

	type ClosedRow = LiveRow & {
		id: number;
		ended_unix: number;
	};

	type ConnListResp = {
		retention_days: number;
		live: LiveRow[];
		closed: ClosedRow[];
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
		total_req: number;
		total_client_event: number;
		series: unknown;
		subscription_details?: SubDetail[];
	};

	let retentionDays = $state(30);
	let liveRows = $state<LiveRow[]>([]);
	let closedRows = $state<ClosedRow[]>([]);
	let listErr = $state<string | null>(null);
	let listLoading = $state(true);

	let selectedRef = $state<string | null>(null);
	let detailSheetOpen = $state(false);
	let detail = $state<ConnDetailResp | null>(null);
	let detailErr = $state<string | null>(null);
	let detailLoading = $state(false);

	let refreshSec = $state<AdminConnRefreshSec>(5);
	let listTimer: ReturnType<typeof setInterval> | null = null;
	let detailTimer: ReturnType<typeof setInterval> | null = null;

	function parseSeries(raw: unknown): SeriesPoint[] {
		if (!Array.isArray(raw)) return [];
		const out: SeriesPoint[] = [];
		for (const x of raw) {
			if (!x || typeof x !== 'object') continue;
			const o = x as Record<string, unknown>;
			const t = Number(o.t);
			const req = Number(o.req);
			const ev = Number(o.ev);
			if (!Number.isFinite(t) || !Number.isFinite(req) || !Number.isFinite(ev)) continue;
			out.push({ t, req, ev });
		}
		return out;
	}

	let detailPts = $derived(detail ? parseSeries(detail.series) : []);

	async function loadList() {
		listErr = null;
		try {
			const res = await adminFetch('/api/audit/connections?limit=100&offset=0');
			if (!res.ok) {
				listErr = `HTTP ${res.status}`;
				return;
			}
			const j = (await res.json()) as ConnListResp;
			retentionDays = typeof j.retention_days === 'number' ? j.retention_days : 30;
			liveRows = Array.isArray(j.live) ? j.live : [];
			closedRows = Array.isArray(j.closed) ? j.closed : [];
		} catch (e) {
			listErr = e instanceof Error ? e.message : 'load failed';
		} finally {
			listLoading = false;
		}
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
		if (listTimer) clearInterval(listTimer);
		listTimer = setInterval(() => {
			if (!selectedRef) void loadList();
		}, refreshSec * 1000);
	}

	function selectRow(ref: string) {
		selectedRef = ref;
		detailSheetOpen = true;
		void loadDetailRef(ref);
		if (detailTimer) clearInterval(detailTimer);
		detailTimer = setInterval(() => {
			if (selectedRef === ref) void loadDetailRef(ref);
		}, refreshSec * 1000);
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
		void loadList();
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
		if (selectedRef) {
			const ref = selectedRef;
			if (detailTimer) clearInterval(detailTimer);
			detailTimer = setInterval(() => {
				if (selectedRef === ref) void loadDetailRef(ref);
			}, refreshSec * 1000);
		}
	});

	onMount(() => {
		refreshSec = readAdminConnRefreshSecFromStorage();
		void loadList();
	});

	onDestroy(() => {
		if (listTimer) clearInterval(listTimer);
		if (detailTimer) clearInterval(detailTimer);
	});

	const selectClass =
		'border-input dark:bg-input/30 focus-visible:border-ring focus-visible:ring-ring/50 h-8 rounded-lg border bg-transparent px-2.5 text-sm shadow-xs outline-none focus-visible:ring-3';
</script>

<div class="space-y-6">
	<AdminPageHeading
		title="Audit · Connections"
		subtitle="Live and recent WebSocket sessions. Retention matches audit log ({retentionDays} days)."
		Icon={Activity}
	/>

	<div class="flex flex-wrap items-end gap-4">
		<div class="space-y-1.5">
			<Label for="conn-refresh">Refresh (seconds)</Label>
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
					<option value={String(s)}>{s}s</option>
				{/each}
			</select>
		</div>
		<Button variant="outline" size="sm" type="button" onclick={() => void loadList()}>Reload list</Button>
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
								<Table.Head class="w-[7.5rem] min-w-[7.5rem] max-w-[9rem]">REQ / EVENT</Table.Head>
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
			<Card.Title class="text-base">Previous</Card.Title>
			<Card.Description>Closed sessions kept for the same retention window as audit log.</Card.Description>
		</Card.Header>
		<Card.Content class="pt-0">
			{#if listLoading}
				<p class="text-muted-foreground py-6 text-sm">Loading…</p>
			{:else if closedRows.length === 0}
				<p class="text-muted-foreground py-6 text-sm">No closed sessions in retention window.</p>
			{:else}
				<div class="overflow-x-auto">
					<Table.Root>
						<Table.Header>
							<Table.Row>
								<Table.Head>Conn</Table.Head>
								<Table.Head>IP</Table.Head>
								<Table.Head class="text-right">Subs*</Table.Head>
								<Table.Head>Duration</Table.Head>
								<Table.Head class="w-[7.5rem] min-w-[7.5rem] max-w-[9rem]">REQ / EVENT</Table.Head>
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
									<Table.Cell class="py-0.5 align-middle">
										<ConnectionReqEventDeltaChart {pts} compact />
									</Table.Cell>
								</Table.Row>
							{/each}
						</Table.Body>
					</Table.Root>
					<p class="text-muted-foreground mt-2 text-xs">
						*Subscription counts for closed rows are shown in the detail panel from the disconnect snapshot.
					</p>
				</div>
			{/if}
		</Card.Content>
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
						<dt class="text-muted-foreground">Totals REQ / EVENT</dt>
						<dd class="tabular-nums">{detail.total_req} / {detail.total_client_event}</dd>
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
