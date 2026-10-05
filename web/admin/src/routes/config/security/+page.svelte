<script lang="ts">
	import Shield from '@lucide/svelte/icons/shield';
	import XIcon from '@lucide/svelte/icons/x';
	import { parseIntSafe } from '$lib/app-config';
	import { relayWebSocketURL } from '$lib/relay-websocket-url';
	import AdminPageHeading from '$lib/components/AdminPageHeading.svelte';
	import { getAdminConfig } from '$lib/config/admin-config-context';
	import * as Card from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Separator } from '$lib/components/ui/separator';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Textarea } from '$lib/components/ui/textarea';

	const ctx = getAdminConfig();

	function draft() {
		return ctx.draft!;
	}

	/** Parsed value from the bound text field; empty or non-numeric → null. */
	function defaultQueryLimitFieldParsed(): number | null {
		const t = ctx.defaultQueryLimitField.trim();
		if (t === '') return null;
		const n = parseInt(t, 10);
		return Number.isFinite(n) ? n : null;
	}

	function defaultQueryLimitFieldShowsNoCapPill(): boolean {
		const n = defaultQueryLimitFieldParsed();
		return n !== null && n < 1;
	}

	/** Parsed value from the bound text field; empty or non-numeric → null. */
	function queryPageSizeFieldParsed(): number | null {
		const t = ctx.queryPageSizeField.trim();
		if (t === '') return null;
		const n = parseInt(t, 10);
		return Number.isFinite(n) ? n : null;
	}

	function queryPageSizeFieldShowsPagingDisabledPill(): boolean {
		const n = queryPageSizeFieldParsed();
		return n !== null && n < 1;
	}

	let aliasDraft = $state('');
	let aliasError = $state<string | null>(null);

	function canonicalURLProblem(): string | null {
		const raw = draft().nip42.relay_url;
		if (raw.trim() === '') return null;
		const parsed = relayWebSocketURL(raw);
		return 'error' in parsed ? parsed.error : null;
	}

	function addAlias() {
		const parsed = relayWebSocketURL(aliasDraft);
		if ('error' in parsed) {
			aliasError = parsed.error;
			return;
		}
		const canonical = relayWebSocketURL(draft().nip42.relay_url);
		const canonicalURL = 'url' in canonical ? canonical.url : '';
		const existing = draft().nip42.relay_aliases.map((alias) => {
			const item = relayWebSocketURL(alias);
			return 'url' in item ? item.url : alias;
		});
		if (parsed.url === canonicalURL || existing.includes(parsed.url)) {
			aliasError = 'That relay URL is already listed.';
			return;
		}
		draft().nip42.relay_aliases = [...draft().nip42.relay_aliases, parsed.url];
		aliasDraft = '';
		aliasError = null;
		ctx.markDirty();
	}

	function removeAlias(url: string) {
		draft().nip42.relay_aliases = draft().nip42.relay_aliases.filter((alias) => alias !== url);
		ctx.markDirty();
	}
</script>

<div class="space-y-8">
	<AdminPageHeading
		title="Security"
		subtitle="Rate limits and NIP-42 client authentication."
		Icon={Shield}
	/>
	<section class="space-y-4">
		<h3 class="text-sm font-medium text-muted-foreground">Rate limits</h3>
		<Card.Root>
			<Card.Content class="grid gap-4 pt-6 sm:grid-cols-2">
				{#each [{ k: 'events_per_minute_per_connection' as const, label: 'Events / min / connection' }, { k: 'bytes_per_second_per_connection' as const, label: 'Bytes / sec / connection' }, { k: 'reqs_per_minute_per_connection' as const, label: 'REQs / min / connection' }, { k: 'messages_per_minute_per_ip' as const, label: 'Messages / min / IP' }] as row (row.k)}
					<div class="space-y-2">
						<Label for={`rl-${row.k}`}>{row.label}</Label>
						<Input
							id={`rl-${row.k}`}
							type="number"
							min="1"
							value={String(draft().rate_limits[row.k])}
							oninput={(e) => {
								draft().rate_limits[row.k] = parseIntSafe(e.currentTarget.value, draft().rate_limits[row.k]);
								ctx.markDirty();
							}}
						/>
					</div>
				{/each}
				<div class="space-y-2">
					<div class="flex flex-wrap items-center gap-2">
						<Label for="default-query-limit">Default query limit</Label>
						{#if defaultQueryLimitFieldShowsNoCapPill()}
							<Badge
								variant="outline"
								class="rounded-full border-amber-500/70 bg-amber-500/15 text-amber-900 dark:border-amber-400/60 dark:bg-amber-950/50 dark:text-amber-100"
							>
								No Limit
							</Badge>
						{/if}
					</div>
					<Input
						id="default-query-limit"
						type="number"
						step="1"
						autocomplete="off"
						spellcheck={false}
						class="font-mono text-sm"
						value={ctx.defaultQueryLimitField}
						oninput={(e) => ctx.setDefaultQueryLimitField(e.currentTarget.value)}
					/>
					{#if ctx.defaultQueryLimitFieldError}
						<div
							role="alert"
							class="rounded-md border border-destructive/80 bg-destructive/10 px-3 py-2 text-sm text-destructive dark:bg-destructive/15"
						>
							{ctx.defaultQueryLimitFieldError}
						</div>
					{/if}
				</div>
				<div class="space-y-2">
					<div class="flex flex-wrap items-center gap-2">
						<Label for="query-page-size">Query page size</Label>
						{#if queryPageSizeFieldShowsPagingDisabledPill()}
							<Badge
								variant="outline"
								class="rounded-full border-amber-500/70 bg-amber-500/15 text-amber-900 dark:border-amber-400/60 dark:bg-amber-950/50 dark:text-amber-100"
							>
								Paging disabled
							</Badge>
						{/if}
					</div>
					<Input
						id="query-page-size"
						type="number"
						step="1"
						autocomplete="off"
						spellcheck={false}
						class="font-mono text-sm"
						value={ctx.queryPageSizeField}
						oninput={(e) => ctx.setQueryPageSizeField(e.currentTarget.value)}
					/>
					{#if ctx.queryPageSizeFieldError}
						<div
							role="alert"
							class="rounded-md border border-destructive/80 bg-destructive/10 px-3 py-2 text-sm text-destructive dark:bg-destructive/15"
						>
							{ctx.queryPageSizeFieldError}
						</div>
					{/if}
				</div>
			</Card.Content>
		</Card.Root>
	</section>

	<Separator />

	<section id="section-nip42" class="space-y-4 scroll-mt-8">
		<h3 class="text-sm font-medium text-muted-foreground">NIP-42 authentication</h3>
		<Card.Root>
			<Card.Header>
				<Card.Title class="text-base">Client authentication</Card.Title>
				<Card.Description>
					Used when NIP-42 is enabled under Enabled NIPs. Set the public WebSocket URL clients put in the
					<code class="rounded bg-muted px-1 text-[0.7rem]">relay</code> tag (for example
					<code class="rounded bg-muted px-1 text-[0.7rem]">wss://relay.example.com/</code>).
					Aliases are other public URLs for the same relay, such as a hosting hostname beside a custom domain.
				</Card.Description>
			</Card.Header>
			<Card.Content class="grid gap-4 pt-0 md:grid-cols-2">
				<div class="space-y-2 md:col-span-2">
					<Label for="nip42-relay-url">Canonical relay URL (ws / wss)</Label>
					<Input
						id="nip42-relay-url"
						class="font-mono text-xs"
						spellcheck={false}
						aria-invalid={canonicalURLProblem() != null}
						value={draft().nip42.relay_url}
						oninput={(e) => {
							draft().nip42.relay_url = e.currentTarget.value;
							ctx.markDirty();
						}}
						onblur={() => {
							const parsed = relayWebSocketURL(draft().nip42.relay_url);
							if ('url' in parsed && parsed.url !== draft().nip42.relay_url) {
								draft().nip42.relay_url = parsed.url;
								ctx.markDirty();
							}
						}}
					/>
					{#if canonicalURLProblem()}
						<p role="alert" class="text-sm text-destructive">{canonicalURLProblem()}</p>
					{/if}
				</div>
				<div class="space-y-2 md:col-span-2">
					<Label for="nip42-alias">Relay URL aliases</Label>
					<p class="text-xs text-muted-foreground">
						Each alias uses the same WebSocket URL rules as the canonical relay URL. Add a URL, or remove one
						that clients no longer use.
					</p>
					{#if draft().nip42.relay_aliases.length > 0}
						<ul class="flex flex-wrap gap-2" aria-label="Relay URL aliases">
							{#each draft().nip42.relay_aliases as alias (alias)}
								<li>
									<Badge variant="secondary" class="h-auto max-w-full gap-1 py-1 font-mono text-xs">
										<span class="truncate">{alias}</span>
										<button
											type="button"
											class="text-muted-foreground hover:text-foreground rounded-full"
											aria-label={`Remove ${alias}`}
											onclick={() => removeAlias(alias)}
										>
											<XIcon />
										</button>
									</Badge>
								</li>
							{/each}
						</ul>
					{/if}
					<div class="flex gap-2">
						<Input
							id="nip42-alias"
							class="font-mono text-xs"
							spellcheck={false}
							placeholder="wss://alias.example/"
							aria-invalid={aliasError != null}
							bind:value={aliasDraft}
							oninput={() => {
								aliasError = null;
							}}
							onkeydown={(e) => {
								if (e.key === 'Enter') {
									e.preventDefault();
									addAlias();
								}
							}}
						/>
						<Button type="button" variant="outline" onclick={addAlias}>Add</Button>
					</div>
					{#if aliasError}
						<p role="alert" class="text-sm text-destructive">{aliasError}</p>
					{/if}
				</div>
				<fieldset
					id="require-auth-on"
					class="scroll-mt-8 space-y-3 rounded-lg border border-border bg-muted/30 px-4 py-3 md:col-span-2"
				>
					<legend class="px-1 text-sm font-medium">Require AUTH on</legend>
					<label class="flex items-start gap-2 text-sm">
						<input
							type="radio"
							name="nip42-require-auth"
							value="protected_kinds"
							checked={draft().nip42.require_auth === 'protected_kinds'}
							onchange={() => {
								draft().nip42.require_auth = 'protected_kinds';
								ctx.markDirty();
							}}
						/>
						<span>
							<span class="font-medium">Protected kinds</span>
							<span class="mt-1 block text-xs text-muted-foreground">
								Ordinary requests stay open. The relay sends
								<code class="rounded bg-muted px-1">AUTH</code> when a subscribe or publish kind in the
								lists below is hit. NIP-11 <code class="rounded bg-muted px-1">auth_required</code> is false.
							</span>
						</span>
					</label>
					<label class="flex items-start gap-2 text-sm">
						<input
							type="radio"
							name="nip42-require-auth"
							value="connect"
							checked={draft().nip42.require_auth === 'connect'}
							onchange={() => {
								draft().nip42.require_auth = 'connect';
								ctx.markDirty();
							}}
						/>
						<span>
							<span class="font-medium">Connect</span>
							<span class="mt-1 block text-xs text-muted-foreground">
								The relay sends <code class="rounded bg-muted px-1">AUTH</code> when the WebSocket opens
								and rejects every command except AUTH until the client authenticates. NIP-11
								<code class="rounded bg-muted px-1">auth_required</code> is true while NIP-42 is enabled.
							</span>
						</span>
					</label>
				</fieldset>
				<div class="space-y-2">
					<Label for="nip42-skew">Created-at skew (seconds)</Label>
					<Input
						id="nip42-skew"
						type="number"
						min="0"
						value={String(draft().nip42.created_at_skew_seconds)}
						oninput={(e) => {
							draft().nip42.created_at_skew_seconds = parseIntSafe(
								e.currentTarget.value,
								draft().nip42.created_at_skew_seconds
							);
							ctx.markDirty();
						}}
					/>
				</div>
				<div class="space-y-2 md:col-span-2">
					<Label for="nip42-sub-kinds">Require auth for subscribe (kinds)</Label>
					<Input
						id="nip42-sub-kinds"
						class="font-mono text-xs"
						spellcheck={false}
						placeholder="e.g. 4, 40"
						value={draft().nip42.require_auth_subscribe_kinds.join(', ')}
						oninput={(e) => {
							draft().nip42.require_auth_subscribe_kinds = e.currentTarget.value
								.split(/[\s,]+/)
								.map((s) => parseInt(s.trim(), 10))
								.filter((n) => Number.isFinite(n));
							ctx.markDirty();
						}}
					/>
				</div>
				<div class="space-y-2 md:col-span-2">
					<Label for="nip42-pub-kinds">Require auth for publish (kinds)</Label>
					<Input
						id="nip42-pub-kinds"
						class="font-mono text-xs"
						spellcheck={false}
						placeholder="e.g. 1"
						value={draft().nip42.require_auth_publish_kinds.join(', ')}
						oninput={(e) => {
							draft().nip42.require_auth_publish_kinds = e.currentTarget.value
								.split(/[\s,]+/)
								.map((s) => parseInt(s.trim(), 10))
								.filter((n) => Number.isFinite(n));
							ctx.markDirty();
						}}
					/>
				</div>
				<div class="space-y-2 md:col-span-2">
					<Label for="nip42-allow">Allowlisted pubkeys (hex, one per line)</Label>
					<Textarea
						id="nip42-allow"
						class="min-h-[100px] font-mono text-xs"
						spellcheck={false}
						value={draft().nip42.allowlisted_pubkeys.join('\n')}
						oninput={(e) => {
							draft().nip42.allowlisted_pubkeys = e.currentTarget.value
								.split('\n')
								.map((s) => s.trim())
								.filter(Boolean);
							ctx.markDirty();
						}}
					/>
				</div>
			</Card.Content>
		</Card.Root>
	</section>
</div>
