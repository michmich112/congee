<script lang="ts">
	import Radio from '@lucide/svelte/icons/radio';
	import CircleHelp from '@lucide/svelte/icons/circle-help';
	import AdminPageHeading from '$lib/components/AdminPageHeading.svelte';
	import ClipCopy from '$lib/components/ClipCopy.svelte';
	import { getAdminConfig } from '$lib/config/admin-config-context';
	import * as Card from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import { Textarea } from '$lib/components/ui/textarea';

	const ctx = getAdminConfig();

	function draft() {
		return ctx.draft!;
	}

	const envLockedTooltip =
		'This process uses CONGEE_INSTANCE_ID from the environment. To change it, update the environment variable and restart the relay; it cannot be edited here.';
</script>

<section class="space-y-4">
	<AdminPageHeading
		title="Relay"
		subtitle="NIP-11 relay information document."
		Icon={Radio}
	/>
	<Card.Root>
		<Card.Content class="grid gap-4 pt-6 md:grid-cols-2">
			<div class="space-y-2 md:col-span-2">
				<Label for="n11-name">Name</Label>
				<Input
					id="n11-name"
					value={draft().nip11.name}
					oninput={(e) => {
						draft().nip11.name = e.currentTarget.value;
						ctx.markDirty();
					}}
				/>
			</div>
			<div class="space-y-2 md:col-span-2">
				<Label for="n11-desc">Description</Label>
				<Textarea
					id="n11-desc"
					rows={3}
					value={draft().nip11.description}
					oninput={(e) => {
						draft().nip11.description = e.currentTarget.value;
						ctx.markDirty();
					}}
				/>
			</div>
			<div class="md:col-span-2 space-y-4 rounded-lg border border-border bg-muted/20 px-4 py-4">
				<div>
					<p class="text-sm font-medium">Relay identity (NIP-11 pubkey)</p>
					<p class="text-muted-foreground mt-1 text-xs">
						Your public key is generated from your private key. The signing key file defaults to
						<code class="rounded bg-muted px-1">relay.secrets.json</code> next to the JSON config (for example
						<code class="rounded bg-muted px-1">/data/config/relay.secrets.json</code> with the default config path), unless
						<code class="rounded bg-muted px-1">RELAY_SECRETS_PATH</code> is set.
					</p>
				</div>
				{#if ctx.relayIdentity}
					<div class="grid gap-4 sm:grid-cols-1">
						<div class="space-y-2">
							<Label for="n11-npub">npub</Label>
							<div class="flex gap-2">
								<Input
									id="n11-npub"
									class="min-w-0 flex-1 bg-muted/50 font-mono text-sm"
									readonly
									tabindex={-1}
									value={ctx.relayIdentity.npub}
								/>
								<ClipCopy
									value={ctx.relayIdentity.npub}
									ariaLabel="Copy npub"
									title="Copy npub"
								/>
							</div>
						</div>
						<div class="space-y-2">
							<Label for="n11-pk">Public key (hex)</Label>
							<div class="flex gap-2">
								<Input
									id="n11-pk"
									class="min-w-0 flex-1 bg-muted/50 font-mono text-xs"
									readonly
									tabindex={-1}
									spellcheck={false}
									value={ctx.relayIdentity.pubkey_hex}
								/>
								<ClipCopy
									value={ctx.relayIdentity.pubkey_hex}
									ariaLabel="Copy public key hex"
									title="Copy public key hex"
								/>
							</div>
						</div>
					</div>
				{:else}
					<p class="text-destructive text-sm">
						Relay identity is not available (same as Dashboard). NIP-11 pubkey cannot be shown; fix relay
						identity or retry after reload.
					</p>
				{/if}
			</div>
			<div class="space-y-2 md:col-span-2 rounded-lg border border-border bg-muted/20 px-4 py-4">
				<div class="flex flex-wrap items-center gap-2">
					<Label for="relay-instance-id" class="text-sm font-medium">Relay instance ID</Label>
					{#if ctx.relayInstanceRuntime?.env_locked}
						<span
							class="inline-flex text-muted-foreground"
							role="img"
							aria-label={envLockedTooltip}
							title={envLockedTooltip}
						>
							<CircleHelp class="size-4" />
						</span>
					{/if}
				</div>
				<p class="text-muted-foreground text-xs">
					Used as the <code class="rounded bg-muted px-1">origin</code> in PostgreSQL
					<code class="rounded bg-muted px-1">LISTEN</code>/<code class="rounded bg-muted px-1">NOTIFY</code> so
					multi-instance relays do not echo their own writes. Changing this requires a relay restart to apply to
					the database listener.
				</p>
				<div class="flex gap-2">
					<Input
						id="relay-instance-id"
						class="min-w-0 flex-1 font-mono text-sm"
						readonly={ctx.relayInstanceRuntime?.env_locked === true}
						tabindex={ctx.relayInstanceRuntime?.env_locked === true ? -1 : undefined}
						value={ctx.relayInstanceRuntime?.env_locked === true
							? (ctx.relayInstanceRuntime?.instance_id ?? '')
							: (draft().relay.instance_id ?? '')}
						oninput={(e) => {
							if (ctx.relayInstanceRuntime?.env_locked === true) return;
							draft().relay.instance_id = e.currentTarget.value;
							ctx.markDirty();
						}}
					/>
					<ClipCopy
						value={ctx.relayInstanceRuntime?.env_locked === true
							? (ctx.relayInstanceRuntime?.instance_id ?? '')
							: (draft().relay.instance_id ?? '')}
						ariaLabel="Copy relay instance ID"
						title="Copy relay instance ID"
					/>
				</div>
				{#if ctx.relayInstanceRuntime?.env_locked}
					<p class="text-muted-foreground text-xs">
						Set via <code class="rounded bg-muted px-1">CONGEE_INSTANCE_ID</code>.
					</p>
				{/if}
			</div>
			<div class="space-y-2">
				<Label for="n11-contact">Contact</Label>
				<Input
					id="n11-contact"
					value={draft().nip11.contact}
					oninput={(e) => {
						draft().nip11.contact = e.currentTarget.value;
						ctx.markDirty();
					}}
				/>
			</div>
			<div class="space-y-2 md:col-span-2 rounded-lg border border-border bg-muted/30 px-4 py-3">
				<p class="text-sm font-medium">Supported NIPs (NIP-11)</p>
				<p class="mt-1 font-mono text-xs text-foreground">
					{[...draft().nips.enabled].sort((a, b) => a - b).join(', ') || '—'}
				</p>
				<p class="mt-2 text-xs text-muted-foreground">
					Mirrors the enabled NIPs list; not stored as a separate config field.
				</p>
			</div>
			<div class="space-y-2">
				<Label for="n11-soft">Software URL</Label>
				<Input
					id="n11-soft"
					class="font-mono text-xs"
					spellcheck={false}
					value={draft().nip11.software}
					oninput={(e) => {
						draft().nip11.software = e.currentTarget.value;
						ctx.markDirty();
					}}
				/>
			</div>
			<div
				class="flex flex-col gap-3 rounded-lg border border-border bg-muted/30 px-4 py-3 md:col-span-2 sm:flex-row sm:items-center sm:justify-between"
			>
				<div class="space-y-1">
					<Label for="n11-cors" class="text-sm font-medium">NIP-11 CORS (any origin)</Label>
					<p class="text-xs text-muted-foreground">
						Sets <code class="rounded bg-muted px-1 text-[0.7rem]">Access-Control-Allow-Origin: *</code> on
						NIP-11 JSON only (GET / with <code class="rounded bg-muted px-1 text-[0.7rem]">Accept:
							application/nostr+json</code>), plus OPTIONS preflight. Also sends
						<code class="rounded bg-muted px-1 text-[0.7rem]">Access-Control-Allow-Private-Network: true</code> so
						public sites (e.g. relay checkers) can reach relays on Tailscale or private IPs (Chrome Private
						Network Access). WebSocket and other responses are unchanged.
					</p>
				</div>
				<Switch
					id="n11-cors"
					checked={draft().nip11.cors_allow_any_origin ?? false}
					onCheckedChange={(on) => {
						draft().nip11.cors_allow_any_origin = on;
						ctx.markDirty();
					}}
				/>
			</div>
		</Card.Content>
	</Card.Root>
</section>
