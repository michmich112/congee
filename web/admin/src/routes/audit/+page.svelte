<script lang="ts">
	import { onMount } from 'svelte';
	import { adminFetch } from '$lib/admin-api';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as Table from '$lib/components/ui/table';

	type Entry = {
		CreatedAt: number;
		Action: string;
		Detail: string;
		Pubkey: string;
	};

	let entries = $state<Entry[]>([]);
	let err = $state<string | null>(null);
	let loading = $state(true);

	let limit = $state('50');
	let offset = $state('0');
	let action = $state('');
	let pubkey = $state('');
	let since = $state('');
	let until = $state('');

	function qs(): string {
		const p = new URLSearchParams();
		if (limit) p.set('limit', limit);
		if (offset) p.set('offset', offset);
		if (action.trim()) p.set('action', action.trim());
		if (pubkey.trim()) p.set('pubkey', pubkey.trim());
		if (since.trim()) p.set('since', since.trim());
		if (until.trim()) p.set('until', until.trim());
		const s = p.toString();
		return s ? `?${s}` : '';
	}

	async function load() {
		loading = true;
		err = null;
		try {
			const r = await adminFetch(`/api/audit${qs()}`);
			if (!r.ok) {
				err = `HTTP ${r.status}`;
				entries = [];
				return;
			}
			const data = (await r.json()) as { entries?: Entry[] };
			entries = data.entries ?? [];
		} catch (e) {
			err = e instanceof Error ? e.message : 'request failed';
			entries = [];
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		void load();
	});
</script>

<div class="space-y-6">
	<div>
		<h2 class="text-xl font-semibold tracking-tight">Audit log</h2>
		<p class="text-sm text-muted-foreground">Newest-first relay activity (full pubkeys in API).</p>
	</div>

	<div class="grid gap-4 rounded-lg border border-border bg-card p-4 sm:grid-cols-2 lg:grid-cols-3">
		<div class="space-y-2">
			<Label for="a-limit">Limit</Label>
			<Input id="a-limit" type="number" min="1" max="500" bind:value={limit} />
		</div>
		<div class="space-y-2">
			<Label for="a-offset">Offset</Label>
			<Input id="a-offset" type="number" min="0" bind:value={offset} />
		</div>
		<div class="space-y-2">
			<Label for="a-action">Action</Label>
			<Input id="a-action" bind:value={action} placeholder="filter" />
		</div>
		<div class="space-y-2 sm:col-span-2">
			<Label for="a-pubkey">Pubkey</Label>
			<Input id="a-pubkey" bind:value={pubkey} placeholder="hex" class="font-mono text-xs" />
		</div>
		<div class="space-y-2">
			<Label for="a-since">Since (unix)</Label>
			<Input id="a-since" bind:value={since} />
		</div>
		<div class="space-y-2">
			<Label for="a-until">Until (unix)</Label>
			<Input id="a-until" bind:value={until} />
		</div>
		<div class="flex items-end">
			<Button type="button" onclick={() => void load()}>Apply filters</Button>
		</div>
	</div>

	{#if err}
		<p class="text-sm text-destructive">{err}</p>
	{/if}

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
					</Table.Row>
				</Table.Header>
				<Table.Body>
					{#each entries as row (row.CreatedAt + row.Action + row.Pubkey + row.Detail)}
						<Table.Row>
							<Table.Cell class="font-mono text-xs tabular-nums">{row.CreatedAt}</Table.Cell>
							<Table.Cell class="text-sm">{row.Action}</Table.Cell>
							<Table.Cell class="max-w-[200px] truncate font-mono text-xs" title={row.Pubkey}
								>{row.Pubkey || '—'}</Table.Cell
							>
							<Table.Cell class="max-w-md truncate text-sm text-muted-foreground" title={row.Detail}
								>{row.Detail || '—'}</Table.Cell
							>
						</Table.Row>
					{:else}
						<Table.Row>
							<Table.Cell colspan={4} class="text-center text-sm text-muted-foreground">No rows</Table.Cell>
						</Table.Row>
					{/each}
				</Table.Body>
			</Table.Root>
		</div>
	{/if}
</div>
