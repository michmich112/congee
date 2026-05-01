<script lang="ts">
	import Network from '@lucide/svelte/icons/network';
	import { parseIntSafe } from '$lib/app-config';
	import AdminPageHeading from '$lib/components/AdminPageHeading.svelte';
	import { getAdminConfig } from '$lib/config/admin-config-context';
	import * as Card from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Separator } from '$lib/components/ui/separator';
	import { Switch } from '$lib/components/ui/switch';

	const ctx = getAdminConfig();

	function draft() {
		return ctx.draft!;
	}
</script>

<div class="space-y-8">
	<AdminPageHeading
		title="Network"
		subtitle="Relay and admin ports, connection limits, and WebSocket behavior."
		Icon={Network}
	/>
	<section class="space-y-4">
		<div class="grid gap-6 md:grid-cols-2">
			<Card.Root>
				<Card.Header>
					<Card.Title class="text-base">Relay</Card.Title>
					<Card.Description>WebSocket listener port for Nostr clients.</Card.Description>
				</Card.Header>
				<Card.Content class="space-y-2">
					<Label for="relay-port">Port</Label>
					<Input
						id="relay-port"
						type="number"
						min="1"
						max="65535"
						value={String(draft().relay.port)}
						oninput={(e) => {
							draft().relay.port = parseIntSafe(e.currentTarget.value, draft().relay.port);
							ctx.markDirty();
						}}
					/>
				</Card.Content>
			</Card.Root>
			<Card.Root>
				<Card.Header>
					<Card.Title class="text-base">Admin HTTP</Card.Title>
					<Card.Description>Port for this admin UI and JSON API.</Card.Description>
				</Card.Header>
				<Card.Content class="space-y-2">
					<Label for="admin-port">Port</Label>
					<Input
						id="admin-port"
						type="number"
						min="1"
						max="65535"
						value={String(draft().admin.port)}
						oninput={(e) => {
							draft().admin.port = parseIntSafe(e.currentTarget.value, draft().admin.port);
							ctx.markDirty();
						}}
					/>
				</Card.Content>
			</Card.Root>
		</div>
	</section>

	<Separator />

	<section class="space-y-4">
		<h3 class="text-sm font-medium text-muted-foreground">Connection &amp; subscription limits</h3>
		<Card.Root>
			<Card.Content class="grid gap-4 pt-6 sm:grid-cols-2">
				{#each [{ k: 'max_open' as const, label: 'Max open connections' }, { k: 'max_subscriptions_per_connection' as const, label: 'Max subscriptions / connection' }, { k: 'max_filters_per_req' as const, label: 'Max filters per REQ' }, { k: 'connections_per_minute_per_ip' as const, label: 'Connections / min / IP' }, { k: 'read_deadline_seconds' as const, label: 'Read deadline (seconds)' }, { k: 'write_deadline_seconds' as const, label: 'Write deadline (seconds)' }] as row (row.k)}
					<div class="space-y-2">
						<Label for={`cl-${row.k}`}>{row.label}</Label>
						<Input
							id={`cl-${row.k}`}
							type="number"
							min="1"
							value={String(draft().connection_limits[row.k])}
							oninput={(e) => {
								draft().connection_limits[row.k] = parseIntSafe(
									e.currentTarget.value,
									draft().connection_limits[row.k]
								);
								ctx.markDirty();
							}}
						/>
					</div>
				{/each}
			</Card.Content>
		</Card.Root>
	</section>

	<Separator />

	<section class="space-y-4">
		<h3 class="text-sm font-medium text-muted-foreground">WebSocket &amp; subscriptions</h3>
		<Card.Root>
			<Card.Content class="grid gap-6 pt-6 md:grid-cols-2">
				<div class="flex items-center justify-between gap-4 rounded-lg border border-border px-4 py-3">
					<div class="space-y-1">
						<p class="text-sm font-medium">Compression</p>
						<p class="text-xs text-muted-foreground">Permessage-deflate where supported.</p>
					</div>
					<Switch
						checked={draft().websocket.compression_enabled}
						onCheckedChange={(on) => {
							draft().websocket.compression_enabled = on;
							ctx.markDirty();
						}}
					/>
				</div>
				<div class="space-y-2">
					<Label for="ws-max">Max message size (bytes)</Label>
					<Input
						id="ws-max"
						type="number"
						min="1"
						value={String(draft().websocket.max_message_bytes)}
						oninput={(e) => {
							draft().websocket.max_message_bytes = parseIntSafe(
								e.currentTarget.value,
								draft().websocket.max_message_bytes
							);
							ctx.markDirty();
						}}
					/>
				</div>
				<div class="space-y-2 md:col-span-2">
					<Label for="sub-id-len">Max subscription id length</Label>
					<Input
						id="sub-id-len"
						type="number"
						min="1"
						value={String(draft().max_subscription_id_length)}
						oninput={(e) => {
							draft().max_subscription_id_length = parseIntSafe(
								e.currentTarget.value,
								draft().max_subscription_id_length
							);
							ctx.markDirty();
						}}
					/>
				</div>
			</Card.Content>
		</Card.Root>
	</section>
</div>
