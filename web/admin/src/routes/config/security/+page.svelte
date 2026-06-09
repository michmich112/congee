<script lang="ts">
	import Shield from '@lucide/svelte/icons/shield';
	import { parseIntSafe } from '$lib/app-config';
	import AdminPageHeading from '$lib/components/AdminPageHeading.svelte';
	import { getAdminConfig } from '$lib/config/admin-config-context';
	import * as Card from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Separator } from '$lib/components/ui/separator';
	import { Switch } from '$lib/components/ui/switch';
	import { Badge } from '$lib/components/ui/badge';
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
				</Card.Description>
			</Card.Header>
			<Card.Content class="grid gap-4 pt-0 md:grid-cols-2">
				<div class="space-y-2 md:col-span-2">
					<Label for="nip42-relay-url">Canonical relay URL (ws / wss)</Label>
					<Input
						id="nip42-relay-url"
						class="font-mono text-xs"
						spellcheck={false}
						value={draft().nip42.relay_url}
						oninput={(e) => {
							draft().nip42.relay_url = e.currentTarget.value;
							ctx.markDirty();
						}}
					/>
				</div>
				<div
					class="flex flex-col gap-3 rounded-lg border border-border bg-muted/30 px-4 py-3 md:col-span-2 sm:flex-row sm:items-center sm:justify-between"
				>
					<div class="space-y-1">
						<Label for="nip42-chal" class="text-sm font-medium">Send AUTH challenge on connect</Label>
						<p class="text-xs text-muted-foreground">
							<strong class="font-medium text-foreground">On:</strong> the relay sends
							<code class="rounded bg-muted px-1 text-[0.7rem]">AUTH</code> with a challenge as soon as the
							WebSocket opens, so clients can authenticate before any gated
							<code class="rounded bg-muted px-1 text-[0.7rem]">REQ</code> or
							<code class="rounded bg-muted px-1 text-[0.7rem]">EVENT</code>.
							<strong class="font-medium text-foreground">Off:</strong> the relay still sends
							<code class="rounded bg-muted px-1 text-[0.7rem]">AUTH</code> immediately before a
							<code class="rounded bg-muted px-1 text-[0.7rem]">CLOSED</code> or
							<code class="rounded bg-muted px-1 text-[0.7rem]">OK</code> that returns
							<code class="rounded bg-muted px-1 text-[0.7rem]">auth-required:</code>, so connections that
							never touch protected kinds avoid an extra message (NIP-42 lazy auth).
						</p>
					</div>
					<Switch
						id="nip42-chal"
						checked={draft().nip42.send_challenge_on_connect}
						onCheckedChange={(on) => {
							draft().nip42.send_challenge_on_connect = on;
							ctx.markDirty();
						}}
					/>
				</div>
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
