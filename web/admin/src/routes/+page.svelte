<script lang="ts">
	import { onMount } from 'svelte';
	import { adminFetch } from '$lib/admin-api';
	import * as Card from '$lib/components/ui/card';
	import { Badge } from '$lib/components/ui/badge';

	type Stats = {
		open_connections?: number;
		relay_port?: number;
		admin_port?: number;
	};

	let stats = $state<Stats | null>(null);
	let loadErr = $state<string | null>(null);
	let loading = $state(true);

	onMount(async () => {
		try {
			const statsRes = await adminFetch('/api/stats');
			if (!statsRes.ok) {
				loadErr = statsRes.status === 401 ? 'Unauthorized' : `HTTP ${statsRes.status}`;
				return;
			}
			stats = (await statsRes.json()) as Stats;
		} catch (e) {
			loadErr = e instanceof Error ? e.message : 'request failed';
		} finally {
			loading = false;
		}
	});
</script>

<div class="space-y-6">
	<div>
		<h2 class="text-xl font-semibold tracking-tight">Dashboard</h2>
		<p class="text-sm text-muted-foreground">Relay and admin listener status.</p>
	</div>

	{#if loading}
		<p class="text-sm text-muted-foreground">Loading stats…</p>
	{:else if loadErr}
		<p class="text-sm text-destructive">{loadErr}</p>
	{:else if stats}
		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
			<Card.Root>
				<Card.Header class="pb-2">
					<Card.Description>Open WebSocket connections</Card.Description>
					<Card.Title class="text-3xl tabular-nums">{stats.open_connections ?? 0}</Card.Title>
				</Card.Header>
				<Card.Content>
					<Badge variant="secondary">live</Badge>
				</Card.Content>
			</Card.Root>
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
