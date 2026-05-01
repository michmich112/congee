<script lang="ts">
	import { onMount } from 'svelte';
	import LayoutDashboard from '@lucide/svelte/icons/layout-dashboard';
	import { adminFetch } from '$lib/admin-api';
	import AdminPageHeading from '$lib/components/AdminPageHeading.svelte';
	import * as Card from '$lib/components/ui/card';
	import { Badge } from '$lib/components/ui/badge';

	type Stats = {
		open_connections?: number;
		relay_port?: number;
		admin_port?: number;
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
</script>

<div class="space-y-6">
	<AdminPageHeading
		title="Dashboard"
		subtitle="Relay and admin listener status."
		Icon={LayoutDashboard}
	/>

	{#if loading}
		<p class="text-sm text-muted-foreground">Loading stats…</p>
	{:else if loadErr}
		<p class="text-sm text-destructive">{loadErr}</p>
	{:else if stats}
		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
			{#if relayIdentity}
				<Card.Root class="sm:col-span-2 lg:col-span-3">
					<Card.Header class="pb-2">
						<Card.Description>Relay identity (read-only)</Card.Description>
						<Card.Title class="text-base font-medium">npub</Card.Title>
					</Card.Header>
					<Card.Content class="space-y-3">
						<p class="break-all font-mono text-sm leading-relaxed">{relayIdentity.npub}</p>
						<div>
							<p class="text-muted-foreground text-xs font-medium tracking-wide uppercase">
								Hex pubkey
							</p>
							<p class="mt-1 break-all font-mono text-sm leading-relaxed">
								{relayIdentity.pubkey_hex}
							</p>
						</div>
					</Card.Content>
				</Card.Root>
			{:else if relayIdErr}
				<Card.Root class="sm:col-span-2 lg:col-span-3">
					<Card.Header class="pb-2">
						<Card.Description>Relay identity</Card.Description>
					</Card.Header>
					<Card.Content>
						<p class="text-destructive text-sm">{relayIdErr}</p>
					</Card.Content>
				</Card.Root>
			{/if}
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
