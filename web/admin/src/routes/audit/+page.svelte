<script lang="ts">
	import { onMount } from 'svelte';
	import { adminFetch } from '$lib/admin-api';
	import { Button } from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as Table from '$lib/components/ui/table';

	type Entry = {
		created_at: number;
		action: string;
		detail: string;
		pubkey: string;
	};

	const eventIDInDetail = /event_id=([0-9a-f]{64})/i;

	function parseAuditEventId(detail: string): string | null {
		const m = detail.match(eventIDInDetail);
		return m ? m[1].toLowerCase() : null;
	}

	let entries = $state<Entry[]>([]);
	let err = $state<string | null>(null);
	let loading = $state(true);

	let limit = $state('50');
	let offset = $state('0');
	let action = $state('');
	let pubkey = $state('');
	let since = $state('');
	let until = $state('');

	let dialogOpen = $state(false);
	let selectedEventId = $state<string | null>(null);
	let eventBody = $state<string | null>(null);
	let eventLoadErr = $state<string | null>(null);
	let eventLoading = $state(false);

	async function load() {
		loading = true;
		err = null;
		try {
			const q = new URLSearchParams();
			if (limit) q.set('limit', limit);
			if (offset) q.set('offset', offset);
			if (since) q.set('since', since);
			if (until) q.set('until', until);
			if (action) q.set('action', action);
			if (pubkey) q.set('pubkey', pubkey);
			const res = await adminFetch(`/api/audit?${q}`);
			if (!res.ok) {
				err = await res.text();
				return;
			}
			const data = (await res.json()) as { entries: Entry[] };
			entries = data.entries ?? [];
		} catch (e) {
			err = e instanceof Error ? e.message : String(e);
		} finally {
			loading = false;
		}
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
			Filter and paginate relay audit entries (newest first). For <code class="text-xs">event_accepted</code> rows
			with an <code class="text-xs">event_id</code> in the detail, open the stored Nostr event in a modal.
		</p>
	</div>

	<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
		<div class="space-y-2">
			<Label for="lim">Limit</Label>
			<Input id="lim" type="number" min="1" max="500" bind:value={limit} />
		</div>
		<div class="space-y-2">
			<Label for="off">Offset</Label>
			<Input id="off" type="number" min="0" bind:value={offset} />
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
			<Button type="button" onclick={() => void load()}>Apply filters</Button>
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
		<div class="overflow-x-auto rounded-lg border border-border">
			<Table.Root>
				<Table.Header>
					<Table.Row>
						<Table.Head class="whitespace-nowrap">Time (unix)</Table.Head>
						<Table.Head>Action</Table.Head>
						<Table.Head>Pubkey</Table.Head>
						<Table.Head>Detail</Table.Head>
						<Table.Head class="w-[100px]">Event</Table.Head>
					</Table.Row>
				</Table.Header>
				<Table.Body>
					{#each entries as row, i (i + '-' + row.created_at + '-' + row.action + '-' + row.detail)}
						{@const eid = parseAuditEventId(row.detail)}
						<Table.Row>
							<Table.Cell class="font-mono text-xs tabular-nums">{row.created_at}</Table.Cell>
							<Table.Cell class="text-sm">{row.action}</Table.Cell>
							<Table.Cell class="max-w-[200px] truncate font-mono text-xs" title={row.pubkey}
								>{row.pubkey || '—'}</Table.Cell
							>
							<Table.Cell class="max-w-md truncate text-sm text-muted-foreground" title={row.detail}
								>{row.detail || '—'}</Table.Cell
							>
							<Table.Cell>
								{#if eid}
									<Button type="button" variant="outline" size="sm" onclick={() => void openEventModal(eid)}>
										View
									</Button>
								{:else}
									<span class="text-xs text-muted-foreground">—</span>
								{/if}
							</Table.Cell>
						</Table.Row>
					{:else}
						<Table.Row>
							<Table.Cell colspan={5} class="text-center text-sm text-muted-foreground">No rows</Table.Cell>
						</Table.Row>
					{/each}
				</Table.Body>
			</Table.Root>
		</div>
	{/if}
</div>
