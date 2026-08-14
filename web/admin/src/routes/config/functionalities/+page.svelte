<script lang="ts">
	import Puzzle from '@lucide/svelte/icons/puzzle';
	import ChevronDown from '@lucide/svelte/icons/chevron-down';
	import { parseIntSafe } from '$lib/app-config';
	import AdminPageHeading from '$lib/components/AdminPageHeading.svelte';
	import StatInfoIcon from '$lib/components/StatInfoIcon.svelte';
	import { getAdminConfig } from '$lib/config/admin-config-context';
	import { Badge } from '$lib/components/ui/badge';
	import * as Card from '$lib/components/ui/card';
	import * as Collapsible from '$lib/components/ui/collapsible';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import { cn } from '$lib/utils';

	const ctx = getAdminConfig();

	let nip17Open = $state(false);
	let nip29Open = $state(false);
	let nip77Open = $state(false);

	const NIP17_REJECT_GIFT_WRAP_INFO =
		'When NIP-17 is off, clients may still publish kind 1059 gift wraps to this relay. With this enabled, those events are rejected before storage and are never relayed to subscribers, so they are not kept without NIP-17 recipient-scoped delivery. Disable only if you intentionally accept gift wraps without NIP-17 privacy rules.';

	function draft() {
		return ctx.draft!;
	}

	const nip17Enabled = $derived(draft().nips.enabled.includes(17));
	const nip29Enabled = $derived(draft().nips.enabled.includes(29));
	const nip77Enabled = $derived(draft().nips.enabled.includes(77));

	function upstreamFiltersText(u: { filters: unknown[] }): string {
		try {
			return JSON.stringify(u.filters ?? [], null, 2);
		} catch {
			return '[]';
		}
	}

	function setUpstreamFilters(idx: number, text: string) {
		try {
			const parsed = JSON.parse(text) as unknown[];
			if (!Array.isArray(parsed)) return;
			draft().nip77.upstreams[idx].filters = parsed;
			ctx.markDirty();
		} catch {
			// keep editing until valid JSON
		}
	}

	function addUpstream() {
		draft().nip77.upstreams.push({
			name: '',
			url: '',
			filters: [{ kinds: [1] }],
			interval_seconds: 3600,
			enabled: false
		});
		ctx.markDirty();
	}

	function removeUpstream(idx: number) {
		draft().nip77.upstreams.splice(idx, 1);
		ctx.markDirty();
	}
</script>

<div class="space-y-6">
	<AdminPageHeading
		title="Functionalities"
		subtitle="Optional NIPs and per-NIP options such as NIP-17 private DMs and NIP-29 relay groups."
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
					{#if nip.number === 17}
						<li class="block">
							<Collapsible.Root bind:open={nip17Open} class="block">
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
													nip17Open ? 'rotate-180' : ''
												)}
											/>
											NIP-17 settings
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
											<p class="text-sm font-medium">NIP-17 private direct messages</p>
											<p class="mt-1 text-xs text-muted-foreground">
												When NIP-17 is enabled, gift wraps (kind 1059) are stored and only delivered to
												NIP-42-authenticated recipients tagged in <code
													class="rounded bg-muted px-1 text-[0.7rem]">p</code
												>. NIP-42 must also be enabled in configuration.
											</p>
										</div>
										{#if nip17Enabled}
											<p class="text-sm text-muted-foreground">
												Reject-when-disabled does not apply while NIP-17 is enabled.
											</p>
										{/if}
										<div
											class="flex flex-col gap-3 rounded-lg border border-border bg-muted/30 px-4 py-3 sm:flex-row sm:items-center sm:justify-between"
										>
											<div class="space-y-1">
												<div class="flex flex-wrap items-center gap-1.5">
													<Label
														for="nip17-reject-gw"
														class="text-sm font-medium {nip17Enabled ? 'opacity-50' : ''}"
													>
														Reject kind 1059 (encrypted DMs) when disabled
													</Label>
													<StatInfoIcon info={NIP17_REJECT_GIFT_WRAP_INFO} />
												</div>
												<p class="text-xs text-muted-foreground">
													Applies only while NIP-17 is off. Default: on.
												</p>
											</div>
											<Switch
												id="nip17-reject-gw"
												disabled={nip17Enabled}
												checked={draft().nip17.reject_gift_wrap_when_disabled}
												aria-label="Reject kind 1059 gift wraps when NIP-17 is disabled"
												onCheckedChange={(on) => {
													draft().nip17.reject_gift_wrap_when_disabled = on;
													ctx.markDirty();
												}}
											/>
										</div>
									</div>
								</Collapsible.Content>
							</Collapsible.Root>
						</li>
					{:else if nip.number === 29}
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
					{:else if nip.number === 77}
						<li class="block">
							<Collapsible.Root bind:open={nip77Open} class="block">
								<div class="flex flex-wrap items-center justify-between gap-4 px-4 py-3">
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
											{#if draft().nips.enabled.includes(nip.number)}
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
													nip77Open ? 'rotate-180' : ''
												)}
											/>
											NIP-77 settings
										</Collapsible.Trigger>
										<Switch
											id="nip-{nip.number}"
											checked={draft().nips.enabled.includes(nip.number)}
											disabled={!nip.implemented && !draft().nips.enabled.includes(nip.number)}
											aria-label="Enable NIP-77 in configuration"
											onCheckedChange={(on) => {
												draft().nips.enabled = ctx.setNipEnabled(draft().nips.enabled, nip.number, on, nip);
												ctx.markDirty();
											}}
										/>
									</div>
								</div>
								<Collapsible.Content>
									<div class="space-y-4 border-t border-border bg-muted/15 px-4 py-4">
										{#if !nip77Enabled}
											<p class="text-sm text-muted-foreground">
												Enable NIP-77 with the toggle above to change these options.
											</p>
										{/if}
										<div class="grid gap-4 md:grid-cols-2">
											<div class="space-y-2">
												<Label for="nip77-max-records">Max records per query</Label>
												<Input
													id="nip77-max-records"
													type="number"
													min="0"
													disabled={!nip77Enabled}
													value={String(draft().nip77.max_records_per_query)}
													oninput={(e) => {
														draft().nip77.max_records_per_query = parseIntSafe(
															e.currentTarget.value,
															draft().nip77.max_records_per_query
														);
														ctx.markDirty();
													}}
												/>
											</div>
											<div class="space-y-2">
												<Label for="nip77-idle">Session idle timeout (seconds)</Label>
												<Input
													id="nip77-idle"
													type="number"
													min="0"
													disabled={!nip77Enabled}
													value={String(draft().nip77.session_idle_timeout_seconds)}
													oninput={(e) => {
														draft().nip77.session_idle_timeout_seconds = parseIntSafe(
															e.currentTarget.value,
															draft().nip77.session_idle_timeout_seconds
														);
														ctx.markDirty();
													}}
												/>
											</div>
											<div class="space-y-2">
												<Label for="nip77-frame">Frame size limit (bytes)</Label>
												<Input
													id="nip77-frame"
													type="number"
													min="4096"
													disabled={!nip77Enabled}
													value={String(draft().nip77.frame_size_limit_bytes)}
													oninput={(e) => {
														draft().nip77.frame_size_limit_bytes = parseIntSafe(
															e.currentTarget.value,
															draft().nip77.frame_size_limit_bytes
														);
														ctx.markDirty();
													}}
												/>
											</div>
											<div class="space-y-2">
												<Label for="nip77-bp">Backpressure REQ queue depth (0 = off)</Label>
												<Input
													id="nip77-bp"
													type="number"
													min="0"
													disabled={!nip77Enabled}
													value={String(draft().nip77.backpressure_req_queue_depth)}
													oninput={(e) => {
														draft().nip77.backpressure_req_queue_depth = parseIntSafe(
															e.currentTarget.value,
															draft().nip77.backpressure_req_queue_depth
														);
														ctx.markDirty();
													}}
												/>
											</div>
										</div>
										<div class="flex flex-wrap gap-6">
											<div class="flex items-center gap-2">
												<Switch
													id="nip77-upstream-enabled"
													disabled={!nip77Enabled}
													checked={draft().nip77.upstream_enabled}
													onCheckedChange={(on) => {
														draft().nip77.upstream_enabled = on;
														ctx.markDirty();
													}}
												/>
												<Label for="nip77-upstream-enabled">Upstream pull sync enabled</Label>
											</div>
											<div class="flex items-center gap-2">
												<Switch
													id="nip77-upstream-pause"
													disabled={!nip77Enabled}
													checked={draft().nip77.upstream_pause_when_busy}
													onCheckedChange={(on) => {
														draft().nip77.upstream_pause_when_busy = on;
														ctx.markDirty();
													}}
												/>
												<Label for="nip77-upstream-pause">Pause upstream when relay busy</Label>
											</div>
										</div>
										<div class="space-y-3">
											<div class="flex items-center justify-between">
												<p class="text-sm font-medium">Upstream relays</p>
												<button
													type="button"
													class="text-sm text-primary hover:underline disabled:opacity-50"
													disabled={!nip77Enabled}
													onclick={addUpstream}>Add upstream</button
												>
											</div>
											{#each draft().nip77.upstreams as upstream, idx (idx)}
												<div class="space-y-2 rounded-lg border border-border bg-muted/30 p-3">
													<div class="grid gap-3 md:grid-cols-2">
														<Input
															placeholder="name"
															disabled={!nip77Enabled}
															value={upstream.name}
															oninput={(e) => {
																upstream.name = e.currentTarget.value;
																ctx.markDirty();
															}}
														/>
														<Input
															placeholder="wss://relay.example/"
															disabled={!nip77Enabled}
															value={upstream.url}
															oninput={(e) => {
																upstream.url = e.currentTarget.value;
																ctx.markDirty();
															}}
														/>
													</div>
													<textarea
														class="min-h-20 w-full rounded-md border border-border bg-background px-3 py-2 font-mono text-xs"
														disabled={!nip77Enabled}
														value={upstreamFiltersText(upstream)}
														oninput={(e) => setUpstreamFilters(idx, e.currentTarget.value)}
													></textarea>
													<div class="flex flex-wrap items-center justify-between gap-2">
														<div class="flex items-center gap-2">
															<Label for="nip77-up-int-{idx}">Interval (seconds)</Label>
															<Input
																id="nip77-up-int-{idx}"
																type="number"
																min="60"
																class="w-28"
																disabled={!nip77Enabled}
																value={String(upstream.interval_seconds)}
																oninput={(e) => {
																	upstream.interval_seconds = parseIntSafe(
																		e.currentTarget.value,
																		upstream.interval_seconds
																	);
																	ctx.markDirty();
																}}
															/>
														</div>
														<div class="flex items-center gap-3">
															<Switch
																disabled={!nip77Enabled}
																checked={upstream.enabled}
																onCheckedChange={(on) => {
																	upstream.enabled = on;
																	ctx.markDirty();
																}}
															/>
															<span class="text-xs text-muted-foreground">Enabled</span>
															<button
																type="button"
																class="text-xs text-destructive hover:underline"
																disabled={!nip77Enabled}
																onclick={() => removeUpstream(idx)}>Remove</button
															>
														</div>
													</div>
												</div>
											{/each}
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
