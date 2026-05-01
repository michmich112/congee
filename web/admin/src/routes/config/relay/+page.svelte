<script lang="ts">
	import Copy from '@lucide/svelte/icons/copy';
	import Radio from '@lucide/svelte/icons/radio';
	import AdminPageHeading from '$lib/components/AdminPageHeading.svelte';
	import { getAdminConfig } from '$lib/config/admin-config-context';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import { Textarea } from '$lib/components/ui/textarea';

	const ctx = getAdminConfig();

	function draft() {
		return ctx.draft!;
	}

	async function copyToClipboard(text: string) {
		try {
			await navigator.clipboard.writeText(text);
		} catch {
			/* clipboard unavailable */
		}
	}
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
						Same <span class="font-mono">npub</span> and hex pubkey as the Dashboard; from
						<span class="font-mono">GET /api/relay-identity</span> (relay signing keys). Shown for NIP-11; not
						editable here. Saving keeps <span class="font-mono">nip11.pubkey</span> in sync with this identity.
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
								<Button
									type="button"
									variant="outline"
									size="icon"
									class="shrink-0"
									onclick={() => void copyToClipboard(ctx.relayIdentity!.npub)}
									aria-label="Copy npub"
									title="Copy npub"
								>
									<Copy class="size-4" />
								</Button>
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
								<Button
									type="button"
									variant="outline"
									size="icon"
									class="shrink-0"
									onclick={() => void copyToClipboard(ctx.relayIdentity!.pubkey_hex)}
									aria-label="Copy public key hex"
									title="Copy public key hex"
								>
									<Copy class="size-4" />
								</Button>
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
