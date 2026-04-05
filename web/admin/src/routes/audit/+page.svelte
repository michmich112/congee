<script lang="ts">
	import { onMount } from 'svelte';
	import { adminFetch } from '$lib/admin-api';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as Table from '$lib/components/ui/table';

	type Entry = {
		created_at: number;
		action: string;
		detail: string;
		pubkey: string;
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

	onMount(() => void load());
</script>

<div class="space-y-6">
	<div>
		<h2 class="text-xl font-semibold tracking-tight">Audit log</h2>
		<p class="text-sm text-muted-foreground">Filter and paginate relay audit entries (newest first).</p>
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
					{#each entries as row (row.created_at + row.action + row.pubkey + row.detail)}
						<Table.Row>
							<Table.Cell class="font-mono text-xs tabular-nums">{row.created_at}</Table.Cell>
							<Table.Cell class="text-sm">{row.action}</Table.Cell>
							<Table.Cell class="max-w-[200px] truncate font-mono text-xs" title={row.pubkey}
								>{row.pubkey || '—'}</Table.Cell
							>
							<Table.Cell class="max-w-md truncate text-sm text-muted-foreground" title={row.detail}
								>{row.detail || '—'}</Table.Cell
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
