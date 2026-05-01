<script lang="ts">
	import Puzzle from '@lucide/svelte/icons/puzzle';
	import ChevronDown from '@lucide/svelte/icons/chevron-down';
	import { parseIntSafe } from '$lib/app-config';
	import AdminPageHeading from '$lib/components/AdminPageHeading.svelte';
	import { getAdminConfig } from '$lib/config/admin-config-context';
	import { Badge } from '$lib/components/ui/badge';
	import * as Card from '$lib/components/ui/card';
	import * as Collapsible from '$lib/components/ui/collapsible';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import { cn } from '$lib/utils';

	const ctx = getAdminConfig();

	let nip29Open = $state(false);

	function draft() {
		return ctx.draft!;
	}

	const nip29Enabled = $derived(draft().nips.enabled.includes(29));
</script>

<div class="space-y-6">
	<AdminPageHeading
		title="Functionalities"
		subtitle="Optional NIPs and per-NIP options such as NIP-29 relay groups."
		Icon={Puzzle}
	/>
	<section id="section-nips" class="space-y-4 scroll-mt-8">
	<Card.Root>
		<Card.Header>
			<Card.Title class="text-base">Enabled NIPs</Card.Title>
			<Card.Description>
				Turn optional protocols on or off here; changes apply when you click Save. Mandatory NIPs stay enabled.
			</Card.Description>
		</Card.Header>
		<Card.Content class="p-0">
			<ul class="divide-y divide-border">
				{#each ctx.nipCatalog as nip (nip.number)}
					{#if nip.number === 29}
						<li class="block">
							<Collapsible.Root bind:open={nip29Open} class="block">
								<div
									class="flex flex-wrap items-center justify-between gap-4 px-4 py-3"
								>
									<div class="min-w-0 flex-1 space-y-1">
										<div class="flex flex-wrap items-center gap-2">
											{#if nip.github_url}
												<a
													href={nip.github_url}
													class="font-mono text-sm font-medium text-primary underline-offset-4 hover:underline"
													target="_blank"
													rel="noreferrer">NIP-{nip.number}</a
												>
											{:else}
												<span class="font-mono text-sm font-medium">NIP-{nip.number}</span>
											{/if}
											{#if nip.mandatory}
												<Badge variant="secondary">mandatory</Badge>
											{:else if draft().nips.enabled.includes(nip.number)}
												<Badge>enabled</Badge>
											{:else}
												<Badge variant="outline">disabled</Badge>
											{/if}
										</div>
										<p class="text-sm text-muted-foreground">{nip.title}</p>
									</div>
									<div class="flex items-center gap-2">
										<Collapsible.Trigger
											class="inline-flex h-9 items-center gap-1.5 rounded-md border border-border bg-background px-3 text-sm font-medium hover:bg-muted/60"
										>
											<ChevronDown
												class={cn(
													'text-muted-foreground size-4 shrink-0 transition-transform duration-200',
													nip29Open ? 'rotate-180' : ''
												)}
											/>
											NIP-29 settings
										</Collapsible.Trigger>
										{#if nip.mandatory}
											<span class="text-sm text-muted-foreground">Always on</span>
										{:else}
											<Switch
												id="nip-{nip.number}"
												checked={draft().nips.enabled.includes(nip.number)}
												disabled={nip.mandatory ||
													(!nip.implemented && !draft().nips.enabled.includes(nip.number))}
												aria-label={nip.mandatory
													? `NIP-${nip.number}, mandatory (always enabled)`
													: `Enable NIP-${nip.number} in configuration`}
												onCheckedChange={(on) => {
													draft().nips.enabled = ctx.setNipEnabled(draft().nips.enabled, nip.number, on, nip);
													ctx.markDirty();
												}}
											/>
										{/if}
									</div>
								</div>
								<Collapsible.Content>
									<div class="space-y-4 border-t border-border bg-muted/15 px-4 py-4">
										<div>
											<p class="text-sm font-medium">NIP-29 relay groups</p>
											<p class="mt-1 text-xs text-muted-foreground">
												Used when NIP-29 is enabled. Private group reads require NIP-42 authentication so the
												relay can match viewers to membership (kind 9000 / 9001 chain).
											</p>
										</div>
										{#if !nip29Enabled}
											<p class="text-sm text-muted-foreground">
												Enable NIP-29 with the toggle above to change these options.
											</p>
										{/if}
										<div class="grid gap-4 md:grid-cols-2">
											<div class="space-y-2">
												<Label for="nip29-late" class={!nip29Enabled ? 'pointer-events-none opacity-50' : ''}
													>Late publication window (seconds)</Label
												>
												<Input
													id="nip29-late"
													type="number"
													min="0"
													disabled={!nip29Enabled}
													value={String(draft().nip29.late_publication_max_past_seconds)}
													oninput={(e) => {
														draft().nip29.late_publication_max_past_seconds = parseIntSafe(
															e.currentTarget.value,
															draft().nip29.late_publication_max_past_seconds
														);
														ctx.markDirty();
													}}
												/>
												<p class="text-xs text-muted-foreground">
													Reject group events whose <code class="rounded bg-muted px-1 text-[0.7rem]"
														>created_at</code
													>
													is older than this vs relay time. Use <span class="font-mono">0</span> for the built-in default
													(86400).
												</p>
											</div>
											<div
												class="flex flex-col gap-3 rounded-lg border border-border bg-muted/30 px-4 py-3 md:col-span-2 sm:flex-row sm:items-center sm:justify-between"
											>
												<div class="space-y-1">
													<Label
														for="nip29-strict-prev"
														class="text-sm font-medium {!nip29Enabled ? 'opacity-50' : ''}"
														>Strict previous scope</Label
													>
													<p class="text-xs text-muted-foreground">
														When enabled, each <code class="rounded bg-muted px-1 text-[0.7rem]">previous</code> prefix
														must resolve to an event in the same group (<code
															class="rounded bg-muted px-1 text-[0.7rem]">h</code
														>
														tag).
													</p>
												</div>
												<Switch
													id="nip29-strict-prev"
													disabled={!nip29Enabled}
													checked={draft().nip29.strict_previous_same_h}
													onCheckedChange={(on) => {
														draft().nip29.strict_previous_same_h = on;
														ctx.markDirty();
													}}
												/>
											</div>
											<div class="space-y-2 md:col-span-2 rounded-lg border border-border bg-muted/30 px-4 py-3">
												<p class="text-sm font-medium">Relay signing identity (read-only)</p>
												<p class="text-xs text-muted-foreground">
													Same keypair as NIP-11 and relay-signed NIP-29 events (<code
														class="rounded bg-muted px-1 text-[0.7rem]">relay.secrets.json</code
													>).
												</p>
												{#if ctx.relayIdentity}
													<p class="mt-2 font-mono text-xs break-all text-foreground">
														<span class="text-muted-foreground">npub</span>
														{ctx.relayIdentity.npub}
													</p>
													<p class="mt-1 font-mono text-xs break-all text-muted-foreground">
														<span class="text-muted-foreground">pubkey</span>
														{ctx.relayIdentity.pubkey_hex}
													</p>
												{:else}
													<p class="mt-2 text-xs text-muted-foreground">
														Not available (check admin API / relay identity file).
													</p>
												{/if}
											</div>
										</div>
									</div>
								</Collapsible.Content>
							</Collapsible.Root>
						</li>
					{:else}
						<li class="flex flex-wrap items-center justify-between gap-4 px-4 py-3">
							<div class="min-w-0 flex-1 space-y-1">
								<div class="flex flex-wrap items-center gap-2">
									{#if nip.github_url}
										<a
											href={nip.github_url}
											class="font-mono text-sm font-medium text-primary underline-offset-4 hover:underline"
											target="_blank"
											rel="noreferrer">NIP-{nip.number}</a
										>
									{:else}
										<span class="font-mono text-sm font-medium">NIP-{nip.number}</span>
									{/if}
									{#if nip.mandatory}
										<Badge variant="secondary">mandatory</Badge>
									{:else if draft().nips.enabled.includes(nip.number)}
										<Badge>enabled</Badge>
									{:else}
										<Badge variant="outline">disabled</Badge>
									{/if}
								</div>
								<p class="text-sm text-muted-foreground">{nip.title}</p>
							</div>
							<div class="flex items-center gap-3">
								{#if nip.mandatory}
									<span class="text-sm text-muted-foreground">Always on</span>
								{/if}
								<Switch
									id="nip-{nip.number}"
									checked={draft().nips.enabled.includes(nip.number)}
									disabled={nip.mandatory || (!nip.implemented && !draft().nips.enabled.includes(nip.number))}
									aria-label={nip.mandatory
										? `NIP-${nip.number}, mandatory (always enabled)`
										: `Enable NIP-${nip.number} in configuration`}
									onCheckedChange={(on) => {
										draft().nips.enabled = ctx.setNipEnabled(draft().nips.enabled, nip.number, on, nip);
										ctx.markDirty();
									}}
								/>
							</div>
						</li>
					{/if}
				{/each}
			</ul>
		</Card.Content>
	</Card.Root>
	</section>
</div>
