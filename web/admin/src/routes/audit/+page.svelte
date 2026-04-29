<script lang="ts">
	import { onMount } from 'svelte';
	import { adminFetch } from '$lib/admin-api';
	import { describeNostrKind } from '$lib/nostr-kind-descriptions';
	import { Button } from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import TableTimestampModeSelect from '$lib/components/TableTimestampModeSelect.svelte';
	import TimestampCell from '$lib/components/TimestampCell.svelte';
	import * as Table from '$lib/components/ui/table';

	type Entry = {
		created_at: number;
		action: string;
		detail: string;
		pubkey: string;
	};

	const PAGE_SIZES = [50, 100, 250, 500, 1000] as const;
	type PageSize = (typeof PAGE_SIZES)[number];

	const selectClass =
		'border-input dark:bg-input/30 focus-visible:border-ring focus-visible:ring-ring/50 h-8 w-full max-w-[140px] rounded-lg border bg-transparent px-2.5 text-sm shadow-xs outline-none focus-visible:ring-3 disabled:cursor-not-allowed disabled:opacity-50';

	const eventIDInDetail = /event_id=([0-9a-f]{64})/i;
	const kindInDetail = /\bkind=(\d+)\b/;

	function parseAuditEventId(detail: string): string | null {
		const m = detail.match(eventIDInDetail);
		return m ? m[1].toLowerCase() : null;
	}

	function parseAuditKind(detail: string): number | null {
		const m = detail.match(kindInDetail);
		if (!m) return null;
		const n = Number.parseInt(m[1], 10);
		return Number.isFinite(n) ? n : null;
	}

	function shortEventId(hex: string): string {
		return hex.length <= 10 ? hex : `${hex.slice(0, 8)}…`;
	}

	function isPageSize(n: number): n is PageSize {
		return (PAGE_SIZES as readonly number[]).includes(n);
	}

	let entries = $state<Entry[]>([]);
	let total = $state(0);
	let err = $state<string | null>(null);
	let loading = $state(true);

	let page = $state(1);
	let pageSize = $state<PageSize>(50);
	let action = $state('');
	let pubkey = $state('');
	let since = $state('');
	let until = $state('');

	let dialogOpen = $state(false);
	let selectedEventId = $state<string | null>(null);
	let eventBody = $state<string | null>(null);
	let eventLoadErr = $state<string | null>(null);
	let eventLoading = $state(false);

	const totalPages = $derived(Math.max(1, Math.ceil(total / pageSize)));

	const rangeLabel = $derived.by(() => {
		if (total === 0) {
			return '0 rows (with current filters)';
		}
		const from = (page - 1) * pageSize + 1;
		const to = Math.min(page * pageSize, total);
		return `Rows ${from}–${to} of ${total}`;
	});

	async function load(allowClampRetry = true) {
		loading = true;
		err = null;
		try {
			const q = new URLSearchParams();
			q.set('limit', String(pageSize));
			q.set('offset', String((page - 1) * pageSize));
			if (since) q.set('since', since);
			if (until) q.set('until', until);
			if (action) q.set('action', action);
			if (pubkey) q.set('pubkey', pubkey);
			const res = await adminFetch(`/api/audit?${q}`);
			if (!res.ok) {
				err = await res.text();
				return;
			}
			const data = (await res.json()) as { entries: Entry[]; total?: number };
			entries = data.entries ?? [];
			total = typeof data.total === 'number' ? data.total : 0;
			const tp = Math.max(1, Math.ceil(total / pageSize));
			if (page > tp) {
				page = tp;
				if (allowClampRetry) {
					await load(false);
				}
				return;
			}
		} catch (e) {
			err = e instanceof Error ? e.message : String(e);
		} finally {
			loading = false;
		}
	}

	async function applyFilters() {
		page = 1;
		await load();
	}

	function goPrev() {
		if (page <= 1) return;
		page -= 1;
		void load();
	}

	function goNext() {
		if (page >= totalPages) return;
		page += 1;
		void load();
	}

	function onPageSizeChange(ev: Event) {
		const v = Number.parseInt((ev.currentTarget as HTMLSelectElement).value, 10);
		pageSize = isPageSize(v) ? v : 50;
		page = 1;
		void load();
	}

	async function openEventModal(eventId: string) {
		selectedEventId = eventId;
		eventBody = null;
		eventLoadErr = null;
		eventLoading = true;
		dialogOpen = true;
		try {
			const res = await adminFetch(`/api/events/${eventId}`);
			if (res.status === 404) {
				eventLoadErr =
					'Event not found in storage (ephemeral kinds are not persisted, or data was deleted). Audit row still shows the id from when it was accepted.';
				return;
			}
			if (!res.ok) {
				const t = await res.text();
				eventLoadErr = t || `HTTP ${res.status}`;
				return;
			}
			const data = (await res.json()) as { event: Record<string, unknown> };
			eventBody = JSON.stringify(data.event, null, 2);
		} catch (e) {
			eventLoadErr = e instanceof Error ? e.message : String(e);
		} finally {
			eventLoading = false;
		}
	}

	$effect(() => {
		if (!dialogOpen) {
			selectedEventId = null;
			eventBody = null;
			eventLoadErr = null;
			eventLoading = false;
		}
	});

	onMount(() => void load());
</script>

<div class="space-y-6">
	<div>
		<h2 class="text-xl font-semibold tracking-tight">Audit log</h2>
		<p class="text-sm text-muted-foreground">
			Filter and paginate relay audit entries (newest first). The API uses <code class="text-xs">limit</code> and
			<code class="text-xs">offset</code> with a matching <code class="text-xs">total</code> count. This UI caps
			page size at 1000. Click a truncated <code class="text-xs">event_id</code> to load stored event JSON when
			available (full id on hover). Hover a <code class="text-xs">kind</code> for a short Nostr description. Use the
			timestamps control above the table for time format.
		</p>
	</div>

	<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
		<div class="space-y-2">
			<Label for="page-size">Events per page</Label>
			<select
				id="page-size"
				class={selectClass}
				value={String(pageSize)}
				onchange={onPageSizeChange}
				aria-label="Events per page"
			>
				{#each PAGE_SIZES as sz}
					<option value={String(sz)}>{sz}</option>
				{/each}
			</select>
		</div>
		<div class="space-y-2">
			<Label for="act">Action</Label>
			<Input id="act" bind:value={action} placeholder="exact match" />
		</div>
		<div class="space-y-2">
			<Label for="pk">Pubkey</Label>
			<Input id="pk" bind:value={pubkey} placeholder="exact hex" />
		</div>
		<div class="space-y-2">
			<Label for="since">Since (unix)</Label>
			<Input id="since" bind:value={since} />
		</div>
		<div class="space-y-2">
			<Label for="until">Until (unix)</Label>
			<Input id="until" bind:value={until} />
		</div>
		<div class="flex items-end">
			<Button type="button" onclick={() => void applyFilters()}>Apply filters</Button>
		</div>
	</div>

	{#if err}
		<p class="text-sm text-destructive">{err}</p>
	{/if}

	<Dialog.Root bind:open={dialogOpen}>
		<Dialog.Content class="max-h-[90vh] overflow-y-auto sm:max-w-3xl">
			<Dialog.Header>
				<Dialog.Title class="font-mono text-sm tracking-normal">
					{selectedEventId ?? 'Event'}
				</Dialog.Title>
				<Dialog.Description>
					Event JSON from relay storage. Ephemeral events and older audit rows without
					<code class="text-xs">event_id=…</code> in the detail cannot be loaded here.
				</Dialog.Description>
			</Dialog.Header>
			<div class="min-h-[120px]">
				{#if eventLoading}
					<p class="text-sm text-muted-foreground">Loading…</p>
				{:else if eventLoadErr}
					<p class="text-sm text-destructive">{eventLoadErr}</p>
				{:else if eventBody}
					<pre
						class="max-h-[55vh] overflow-auto rounded-md border border-border bg-muted/40 p-3 font-mono text-xs whitespace-pre-wrap break-all"
						>{eventBody}</pre
					>
				{/if}
			</div>
		</Dialog.Content>
	</Dialog.Root>

	{#if loading}
		<p class="text-sm text-muted-foreground">Loading…</p>
	{:else}
		<div class="overflow-hidden rounded-lg border border-border">
			<div
				class="flex flex-wrap items-center justify-end gap-3 border-b border-border bg-muted/30 px-3 py-2"
			>
				<TableTimestampModeSelect selectId="audit-table-timestamps" />
			</div>
			<div class="overflow-x-auto">
				<Table.Root>
					<Table.Header>
						<Table.Row>
							<Table.Head class="whitespace-nowrap">Time</Table.Head>
							<Table.Head>Action</Table.Head>
							<Table.Head class="whitespace-nowrap text-right">Kind</Table.Head>
							<Table.Head class="whitespace-nowrap">Event ID</Table.Head>
							<Table.Head>Pubkey</Table.Head>
							<Table.Head>Detail</Table.Head>
						</Table.Row>
					</Table.Header>
					<Table.Body>
						{#each entries as row, i (i + '-' + row.created_at + '-' + row.action + '-' + row.detail)}
							{@const eid = parseAuditEventId(row.detail)}
							{@const kind = parseAuditKind(row.detail)}
							<Table.Row>
								<Table.Cell><TimestampCell unixValue={row.created_at} /></Table.Cell>
								<Table.Cell class="text-sm">{row.action}</Table.Cell>
								<Table.Cell class="text-right font-mono text-xs tabular-nums">
									{#if kind !== null}
										<span class="cursor-help border-b border-dotted border-muted-foreground/60" title={describeNostrKind(kind)}
											>{kind}</span
										>
									{:else}
										<span class="text-muted-foreground">—</span>
									{/if}
								</Table.Cell>
								<Table.Cell class="font-mono text-xs">
									{#if eid}
										<button
											type="button"
											class="cursor-pointer text-left text-primary underline-offset-2 hover:underline"
											title={eid}
											onclick={() => void openEventModal(eid)}
										>
											{shortEventId(eid)}
										</button>
									{:else}
										<span class="text-muted-foreground">—</span>
									{/if}
								</Table.Cell>
								<Table.Cell class="max-w-[200px] truncate font-mono text-xs" title={row.pubkey}
									>{row.pubkey || '—'}</Table.Cell
								>
								<Table.Cell class="max-w-md truncate text-sm text-muted-foreground" title={row.detail}
									>{row.detail || '—'}</Table.Cell
								>
							</Table.Row>
						{:else}
							<Table.Row>
								<Table.Cell colspan={6} class="text-center text-sm text-muted-foreground">No rows</Table.Cell>
							</Table.Row>
						{/each}
					</Table.Body>
				</Table.Root>
			</div>
			<div
				class="flex flex-col gap-3 border-t border-border bg-muted/20 px-3 py-3 sm:flex-row sm:items-center sm:justify-between"
			>
				<p class="text-sm text-muted-foreground">{rangeLabel}</p>
				<div class="flex flex-wrap items-center gap-2">
					<Button type="button" variant="outline" size="sm" disabled={page <= 1} onclick={goPrev}>
						Previous
					</Button>
					<span class="text-sm tabular-nums text-muted-foreground">Page {page} / {totalPages}</span>
					<Button type="button" variant="outline" size="sm" disabled={page >= totalPages} onclick={goNext}>
						Next
					</Button>
				</div>
			</div>
		</div>
	{/if}
</div>
