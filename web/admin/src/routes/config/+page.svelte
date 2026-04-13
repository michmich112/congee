<script lang="ts">
	import { onMount } from 'svelte';
	import Code from '@lucide/svelte/icons/code';
	import ChevronDown from '@lucide/svelte/icons/chevron-down';
	import { adminFetch } from '$lib/admin-api';
	import { parseConfigJson, parseIntSafe, type AppConfig } from '$lib/app-config';
	import * as Alert from '$lib/components/ui/alert';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Separator } from '$lib/components/ui/separator';
	import { Switch } from '$lib/components/ui/switch';
	import { Textarea } from '$lib/components/ui/textarea';
	import TableTimestampModeSelect from '$lib/components/TableTimestampModeSelect.svelte';
	import TimestampCell from '$lib/components/TimestampCell.svelte';
	import * as Table from '$lib/components/ui/table';

	type ChangelogRow = {
		created_at: number;
		summary: string;
		json_diff: string;
	};

	type NipRow = {
		number: number;
		title: string;
		github_url: string;
		mandatory: boolean;
		implemented: boolean;
		enabled: boolean;
	};

	const selectClass =
		'border-input dark:bg-input/30 focus-visible:border-ring focus-visible:ring-ring/50 h-8 w-full rounded-lg border bg-transparent px-2.5 text-sm shadow-xs outline-none focus-visible:ring-3 disabled:cursor-not-allowed disabled:opacity-50';

	let draft = $state<AppConfig | null>(null);
	let nipCatalog = $state<NipRow[]>([]);
	let changelog = $state<ChangelogRow[]>([]);
	let loadErr = $state<string | null>(null);
	let changelogErr = $state<string | null>(null);
	let saveErr = $state<string | null>(null);
	let saveOk = $state(false);
	let saveMessage = $state<string | null>(null);
	let loading = $state(true);
	let changelogLoading = $state(false);
	let saving = $state(false);
	let dirty = $state(false);
	let rawOpen = $state(false);
	let rawText = $state('');
	let rawErr = $state<string | null>(null);
	let changelogExpanded = $state(false);
	let relayVersion = $state<string | null>(null);

	function markDirty() {
		dirty = true;
		saveOk = false;
		saveMessage = null;
	}

	function setNipEnabled(list: number[], nip: number, on: boolean, row: NipRow): number[] {
		if (row.mandatory) return list;
		if (!row.implemented && on) {
			saveErr = `NIP-${nip} is not implemented in this relay yet; enable it only via raw JSON if you know what you are doing.`;
			return list;
		}
		const s = new Set(list);
		if (on) s.add(nip);
		else s.delete(nip);
		const next = Array.from(s).sort((a, b) => a - b);
		saveErr = null;
		return next;
	}

	async function loadNipCatalog() {
		try {
			const r = await adminFetch('/api/nips');
			if (!r.ok) {
				nipCatalog = [];
				return;
			}
			const data = (await r.json()) as { nips?: NipRow[] };
			nipCatalog = data.nips ?? [];
		} catch {
			nipCatalog = [];
		}
	}

	async function loadAll() {
		loading = true;
		loadErr = null;
		changelogErr = null;
		saveOk = false;
		saveMessage = null;
		saveErr = null;
		relayVersion = null;
		try {
			const statsRes = await adminFetch('/api/stats');
			if (statsRes.ok) {
				const st = (await statsRes.json()) as { relay_version?: string };
				relayVersion = st.relay_version ?? null;
			}
		} catch {
			relayVersion = null;
		}
		try {
			const cfgRes = await adminFetch('/api/config');
			if (!cfgRes.ok) {
				loadErr = `config: HTTP ${cfgRes.status}`;
				return;
			}
			const text = await cfgRes.text();
			draft = parseConfigJson(text);
			dirty = false;
			await loadNipCatalog();
		} catch (e) {
			loadErr = e instanceof Error ? e.message : 'load failed';
		} finally {
			loading = false;
		}

		changelogLoading = true;
		try {
			const chRes = await adminFetch('/api/config/changelog?limit=50');
			if (!chRes.ok) {
				changelogErr = `changelog: HTTP ${chRes.status}`;
				changelog = [];
				return;
			}
			const ch = (await chRes.json()) as { changelog?: ChangelogRow[] };
			changelog = ch.changelog ?? [];
		} catch (e) {
			changelogErr = e instanceof Error ? e.message : 'changelog load failed';
			changelog = [];
		} finally {
			changelogLoading = false;
		}

		if (typeof window !== 'undefined' && window.location.hash) {
			requestAnimationFrame(() => {
				document.querySelector(window.location.hash)?.scrollIntoView({ behavior: 'smooth' });
			});
		}
	}

	function confirmReload() {
		if (dirty && !window.confirm('Discard unsaved changes and reload from the server?')) return;
		void loadAll();
	}

	function openRawEditor() {
		if (!draft) return;
		rawErr = null;
		rawText = JSON.stringify(draft, null, 2);
		rawOpen = true;
	}

	function applyRawEditor() {
		rawErr = null;
		try {
			const next = parseConfigJson(rawText);
			draft = next;
			markDirty();
			rawOpen = false;
		} catch (e) {
			rawErr = e instanceof Error ? e.message : 'invalid json';
		}
	}

	async function save() {
		if (!draft) return;
		saveErr = null;
		saveOk = false;
		saveMessage = null;
		saving = true;
		try {
			const body = JSON.stringify(draft, null, 2);
			const r = await adminFetch('/api/config', {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body
			});
			const text = await r.text();
			if (!r.ok) {
				try {
					const j = JSON.parse(text) as { error?: string };
					saveErr = j.error ?? text;
				} catch {
					saveErr = text || `HTTP ${r.status}`;
				}
				return;
			}
			let restarting = false;
			try {
				const j = JSON.parse(text) as { restarting?: boolean; restart_required?: boolean };
				restarting = j.restarting === true;
				if (j.restart_required && !restarting) {
					saveMessage =
						'Configuration file was updated. Restart the relay process manually to apply in-memory settings.';
				} else if (restarting) {
					saveMessage = 'Configuration saved. The relay is restarting…';
				} else {
					saveMessage = 'Configuration saved (no changes detected).';
				}
			} catch {
				saveMessage = 'Configuration saved.';
			}
			saveOk = true;
			dirty = false;
			await loadAll();
		} catch (e) {
			saveErr = e instanceof Error ? e.message : 'save failed';
		} finally {
			saving = false;
		}
	}

	onMount(() => {
		void loadAll();
	});
</script>

<div class="space-y-6">
	<div class="space-y-2">
		<div class="flex flex-row flex-wrap items-center gap-x-4 gap-y-2">
			<h2 class="min-w-0 flex-1 text-xl font-semibold tracking-tight">Configuration</h2>
			<div class="flex shrink-0 flex-wrap items-center justify-end gap-2">
				<Button
					type="button"
					variant="outline"
					size="icon"
					disabled={loading || saving || !draft}
					onclick={openRawEditor}
					title="View and edit raw JSON (advanced)"
				>
					<Code class="size-4" />
					<span class="sr-only">Raw JSON</span>
				</Button>
				<Button type="button" variant="outline" disabled={loading || saving} onclick={confirmReload}>
					Reload
				</Button>
				<Button type="button" disabled={loading || saving || !draft || !dirty} onclick={() => void save()}>
					{saving ? 'Saving…' : 'Save'}
				</Button>
			</div>
		</div>
		<p class="text-sm text-muted-foreground">
			Edit settings below, then use <span class="font-medium">Save</span> to write the file and apply changes.
			When a full relay restart is required, saving triggers it automatically.
		</p>
	</div>

	{#if loadErr}
		<p class="text-sm text-destructive">{loadErr}</p>
	{/if}
	{#if changelogErr}
		<p class="text-sm text-destructive">{changelogErr}</p>
	{/if}
	{#if saveErr}
		<p class="text-sm text-destructive">{saveErr}</p>
	{/if}
	{#if saveOk && saveMessage}
		<p class="text-sm text-green-600 dark:text-green-400">{saveMessage}</p>
	{/if}
	{#if dirty}
		<Alert.Root variant="default" class="border-amber-500/50 bg-amber-500/10">
			<Alert.Title>Unsaved changes</Alert.Title>
			<Alert.Description>Click Save to write the configuration file and apply it.</Alert.Description>
		</Alert.Root>
	{/if}

	{#if loading || !draft}
		<p class="text-sm text-muted-foreground">Loading…</p>
	{:else}
		<div class="space-y-8">
			<section class="space-y-4">
				<h3 class="text-sm font-medium text-muted-foreground">Network</h3>
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
								value={String(draft.relay.port)}
								oninput={(e) => {
									draft!.relay.port = parseIntSafe(e.currentTarget.value, draft!.relay.port);
									markDirty();
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
								value={String(draft.admin.port)}
								oninput={(e) => {
									draft!.admin.port = parseIntSafe(e.currentTarget.value, draft!.admin.port);
									markDirty();
								}}
							/>
						</Card.Content>
					</Card.Root>
				</div>
			</section>

			<Separator />

			<section class="space-y-4">
				<h3 class="text-sm font-medium text-muted-foreground">Database</h3>
				<Card.Root>
					<Card.Header>
						<Card.Title class="text-base">Storage</Card.Title>
						<Card.Description>SQLite for single-node; PostgreSQL for larger deployments.</Card.Description>
					</Card.Header>
					<Card.Content class="grid gap-4 md:grid-cols-2">
						<div class="space-y-2 md:col-span-1">
							<Label for="db-type">Type</Label>
							<select
								id="db-type"
								class={selectClass}
								value={draft.database.type}
								onchange={(e) => {
									draft!.database.type = e.currentTarget.value;
									markDirty();
								}}
							>
								<option value="sqlite">sqlite</option>
								<option value="postgres">postgres</option>
							</select>
						</div>
						<div class="space-y-2 md:col-span-2">
							<Label for="db-dsn">DSN</Label>
							<Input
								id="db-dsn"
								class="font-mono text-xs"
								spellcheck={false}
								value={draft.database.dsn}
								oninput={(e) => {
									draft!.database.dsn = e.currentTarget.value;
									markDirty();
								}}
							/>
						</div>
					</Card.Content>
				</Card.Root>
			</section>

			<Separator />

			<section class="space-y-4">
				<h3 class="text-sm font-medium text-muted-foreground">Logging &amp; audit</h3>
				<Card.Root>
					<Card.Content class="grid gap-4 pt-6 md:grid-cols-3">
						<div class="space-y-2">
							<Label for="log-level">Log level</Label>
							<select
								id="log-level"
								class={selectClass}
								value={draft.logging.level}
								onchange={(e) => {
									draft!.logging.level = e.currentTarget.value;
									markDirty();
								}}
							>
								<option value="debug">debug</option>
								<option value="info">info</option>
								<option value="warn">warn</option>
								<option value="error">error</option>
							</select>
						</div>
						<div class="space-y-2">
							<Label for="log-format">Format</Label>
							<select
								id="log-format"
								class={selectClass}
								value={draft.logging.format}
								onchange={(e) => {
									draft!.logging.format = e.currentTarget.value;
									markDirty();
								}}
							>
								<option value="json">json</option>
								<option value="console">console</option>
							</select>
						</div>
						<div class="space-y-2">
							<Label for="audit-days">Audit retention (days)</Label>
							<Input
								id="audit-days"
								type="number"
								min="1"
								value={String(draft.audit.retention_days)}
								oninput={(e) => {
									draft!.audit.retention_days = parseIntSafe(
										e.currentTarget.value,
										draft!.audit.retention_days
									);
									markDirty();
								}}
							/>
						</div>
					</Card.Content>
				</Card.Root>
			</section>

			<Separator />

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
									value={String(draft.rate_limits[row.k])}
									oninput={(e) => {
										draft!.rate_limits[row.k] = parseIntSafe(e.currentTarget.value, draft!.rate_limits[row.k]);
										markDirty();
									}}
								/>
							</div>
						{/each}
					</Card.Content>
				</Card.Root>
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
									value={String(draft.connection_limits[row.k])}
									oninput={(e) => {
										draft!.connection_limits[row.k] = parseIntSafe(
											e.currentTarget.value,
											draft!.connection_limits[row.k]
										);
										markDirty();
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
								checked={draft.websocket.compression_enabled}
								onCheckedChange={(on) => {
									draft!.websocket.compression_enabled = on;
									markDirty();
								}}
							/>
						</div>
						<div class="space-y-2">
							<Label for="ws-max">Max message size (bytes)</Label>
							<Input
								id="ws-max"
								type="number"
								min="1"
								value={String(draft.websocket.max_message_bytes)}
								oninput={(e) => {
									draft!.websocket.max_message_bytes = parseIntSafe(
										e.currentTarget.value,
										draft!.websocket.max_message_bytes
									);
									markDirty();
								}}
							/>
						</div>
						<div class="space-y-2 md:col-span-2">
							<Label for="sub-id-len">Max subscription id length</Label>
							<Input
								id="sub-id-len"
								type="number"
								min="1"
								value={String(draft.max_subscription_id_length)}
								oninput={(e) => {
									draft!.max_subscription_id_length = parseIntSafe(
										e.currentTarget.value,
										draft!.max_subscription_id_length
									);
									markDirty();
								}}
							/>
						</div>
					</Card.Content>
				</Card.Root>
			</section>

			<Separator />

			<section class="space-y-4">
				<h3 class="text-sm font-medium text-muted-foreground">NIP-11 relay information</h3>
				<Card.Root>
					<Card.Content class="grid gap-4 pt-6 md:grid-cols-2">
						<div class="space-y-2 md:col-span-2">
							<Label for="n11-name">Name</Label>
							<Input
								id="n11-name"
								value={draft.nip11.name}
								oninput={(e) => {
									draft!.nip11.name = e.currentTarget.value;
									markDirty();
								}}
							/>
						</div>
						<div class="space-y-2 md:col-span-2">
							<Label for="n11-desc">Description</Label>
							<Textarea
								id="n11-desc"
								rows={3}
								value={draft.nip11.description}
								oninput={(e) => {
									draft!.nip11.description = e.currentTarget.value;
									markDirty();
								}}
							/>
						</div>
						<div class="space-y-2">
							<Label for="n11-pk">Public key (hex)</Label>
							<Input
								id="n11-pk"
								class="font-mono text-xs"
								spellcheck={false}
								value={draft.nip11.pubkey}
								oninput={(e) => {
									draft!.nip11.pubkey = e.currentTarget.value;
									markDirty();
								}}
							/>
						</div>
						<div class="space-y-2">
							<Label for="n11-contact">Contact</Label>
							<Input
								id="n11-contact"
								value={draft.nip11.contact}
								oninput={(e) => {
									draft!.nip11.contact = e.currentTarget.value;
									markDirty();
								}}
							/>
						</div>
						<div class="space-y-2 md:col-span-2 rounded-lg border border-border bg-muted/30 px-4 py-3">
							<p class="text-sm font-medium">Supported NIPs (NIP-11)</p>
							<p class="mt-1 font-mono text-xs text-foreground">
								{[...draft.nips.enabled].sort((a, b) => a - b).join(', ') || '—'}
							</p>
							<p class="mt-2 text-xs text-muted-foreground">
								Mirrors the enabled NIPs list; not stored as a separate config field.
							</p>
						</div>
						<div class="space-y-2 md:col-span-2 rounded-lg border border-border bg-muted/30 px-4 py-3">
							<p class="text-sm font-medium">Relay version (NIP-11)</p>
							<p class="mt-1 font-mono text-sm text-foreground">{relayVersion ?? '—'}</p>
							<p class="mt-2 text-xs text-muted-foreground">
								Comes from the running binary (set at build time with <code class="rounded bg-muted px-1 text-[0.7rem]">go build -ldflags</code>); not in the JSON config file.
							</p>
						</div>
						<div class="space-y-2">
							<Label for="n11-soft">Software URL</Label>
							<Input
								id="n11-soft"
								class="font-mono text-xs"
								spellcheck={false}
								value={draft.nip11.software}
								oninput={(e) => {
									draft!.nip11.software = e.currentTarget.value;
									markDirty();
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
								checked={draft.nip11.cors_allow_any_origin ?? false}
								onCheckedChange={(on) => {
									draft!.nip11.cors_allow_any_origin = on;
									markDirty();
								}}
							/>
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
								value={draft.nip42.relay_url}
								oninput={(e) => {
									draft!.nip42.relay_url = e.currentTarget.value;
									markDirty();
								}}
							/>
						</div>
						<div
							class="flex flex-col gap-3 rounded-lg border border-border bg-muted/30 px-4 py-3 md:col-span-2 sm:flex-row sm:items-center sm:justify-between"
						>
							<div class="space-y-1">
								<Label for="nip42-chal" class="text-sm font-medium">Send AUTH challenge on connect</Label>
								<p class="text-xs text-muted-foreground">
									When enabled, the relay sends <code class="rounded bg-muted px-1 text-[0.7rem]">AUTH</code> with
									a challenge as soon as the WebSocket opens.
								</p>
							</div>
							<Switch
								id="nip42-chal"
								checked={draft.nip42.send_challenge_on_connect}
								onCheckedChange={(on) => {
									draft!.nip42.send_challenge_on_connect = on;
									markDirty();
								}}
							/>
						</div>
						<div class="space-y-2">
							<Label for="nip42-skew">Created-at skew (seconds)</Label>
							<Input
								id="nip42-skew"
								type="number"
								min="0"
								value={String(draft.nip42.created_at_skew_seconds)}
								oninput={(e) => {
									draft!.nip42.created_at_skew_seconds = parseIntSafe(
										e.currentTarget.value,
										draft!.nip42.created_at_skew_seconds
									);
									markDirty();
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
								value={draft.nip42.require_auth_subscribe_kinds.join(', ')}
								oninput={(e) => {
									draft!.nip42.require_auth_subscribe_kinds = e.currentTarget.value
										.split(/[\s,]+/)
										.map((s) => parseInt(s.trim(), 10))
										.filter((n) => Number.isFinite(n));
									markDirty();
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
								value={draft.nip42.require_auth_publish_kinds.join(', ')}
								oninput={(e) => {
									draft!.nip42.require_auth_publish_kinds = e.currentTarget.value
										.split(/[\s,]+/)
										.map((s) => parseInt(s.trim(), 10))
										.filter((n) => Number.isFinite(n));
									markDirty();
								}}
							/>
						</div>
						<div class="space-y-2 md:col-span-2">
							<Label for="nip42-allow">Allowlisted pubkeys (hex, one per line)</Label>
							<Textarea
								id="nip42-allow"
								class="min-h-[100px] font-mono text-xs"
								spellcheck={false}
								value={draft.nip42.allowlisted_pubkeys.join('\n')}
								oninput={(e) => {
									draft!.nip42.allowlisted_pubkeys = e.currentTarget.value
										.split('\n')
										.map((s) => s.trim())
										.filter(Boolean);
									markDirty();
								}}
							/>
						</div>
					</Card.Content>
				</Card.Root>
			</section>

			<Separator />

			<section id="section-nips" class="space-y-4 scroll-mt-8">
				<h3 class="text-sm font-medium text-muted-foreground">NIPs</h3>
				<Card.Root>
					<Card.Header>
						<Card.Title class="text-base">Enabled NIPs</Card.Title>
						<Card.Description>
							Turn optional protocols on or off here; changes apply when you click Save. Mandatory NIPs stay
							enabled.
						</Card.Description>
					</Card.Header>
					<Card.Content class="p-0">
						<ul class="divide-y divide-border">
							{#each nipCatalog as nip (nip.number)}
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
											{:else if draft.nips.enabled.includes(nip.number)}
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
											checked={draft.nips.enabled.includes(nip.number)}
											disabled={nip.mandatory || (!nip.implemented && !draft.nips.enabled.includes(nip.number))}
											aria-label={nip.mandatory
												? `NIP-${nip.number}, mandatory (always enabled)`
												: `Enable NIP-${nip.number} in configuration`}
											onCheckedChange={(on) => {
												draft!.nips.enabled = setNipEnabled(draft!.nips.enabled, nip.number, on, nip);
												markDirty();
											}}
										/>
									</div>
								</li>
							{/each}
						</ul>
					</Card.Content>
				</Card.Root>
			</section>
		</div>

		<details
			class="bg-card text-card-foreground mt-10 rounded-xl border border-border shadow-sm"
			bind:open={changelogExpanded}
		>
			<summary
				class="flex cursor-pointer list-none items-center gap-3 px-6 py-4 marker:hidden [&::-webkit-details-marker]:hidden hover:bg-muted/40"
			>
				<ChevronDown
					class="text-muted-foreground size-4 shrink-0 transition-transform duration-200 {changelogExpanded
						? 'rotate-180'
						: ''}"
				/>
				<div class="min-w-0 flex-1 text-left">
					<p class="text-base font-semibold">Config changelog</p>
					<p class="text-sm text-muted-foreground">
						Recent writes from the admin API (newest first). Use the timestamps control above the table for
						created times.
					</p>
				</div>
			</summary>
			<div class="border-t border-border">
				{#if changelogLoading}
					<p class="px-6 py-4 text-sm text-muted-foreground">Loading changelog…</p>
				{:else}
					<div class="overflow-hidden">
						<div
							class="flex flex-wrap items-center justify-end gap-3 border-b border-border bg-muted/30 px-3 py-2"
						>
							<TableTimestampModeSelect selectId="config-changelog-timestamps" />
						</div>
						<div class="overflow-x-auto">
							<Table.Root>
								<Table.Header>
									<Table.Row>
										<Table.Head class="whitespace-nowrap">Created</Table.Head>
										<Table.Head>Summary</Table.Head>
										<Table.Head>Payload / diff</Table.Head>
									</Table.Row>
								</Table.Header>
								<Table.Body>
									{#each changelog as row, i (`${row.created_at}-${i}`)}
										<Table.Row>
											<Table.Cell><TimestampCell unixValue={row.created_at} /></Table.Cell>
											<Table.Cell class="text-sm">{row.summary}</Table.Cell>
											<Table.Cell
												class="max-w-lg whitespace-pre-wrap break-all font-mono text-xs text-muted-foreground"
												>{row.json_diff}</Table.Cell
											>
										</Table.Row>
									{:else}
										<Table.Row>
											<Table.Cell colspan={3} class="text-center text-sm text-muted-foreground"
												>No entries yet</Table.Cell
											>
										</Table.Row>
									{/each}
								</Table.Body>
							</Table.Root>
						</div>
					</div>
				{/if}
			</div>
		</details>
	{/if}
</div>

{#if rawOpen}
	<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
		role="presentation"
		onclick={(e) => {
			if (e.target === e.currentTarget) rawOpen = false;
		}}
		onkeydown={(e) => {
			if (e.key === 'Escape') rawOpen = false;
		}}
	>
		<div
			class="bg-background max-h-[90vh] w-full max-w-3xl overflow-hidden rounded-xl border border-border shadow-lg"
			role="dialog"
			aria-modal="true"
			aria-labelledby="raw-json-title"
			tabindex="-1"
		>
			<div class="max-h-[90vh] overflow-y-auto">
				<div class="border-b border-border px-6 py-4">
					<h2 id="raw-json-title" class="text-lg font-semibold">Raw JSON</h2>
					<p class="mt-1 text-sm text-muted-foreground">
						For advanced users only. Invalid JSON will be rejected when you save to the server. Prefer the form
						unless you know the schema.
					</p>
				</div>
				<div class="space-y-3 px-6 py-4">
					<Alert.Root variant="default" class="border-destructive/40 bg-destructive/5">
						<Alert.Title>Advanced</Alert.Title>
						<Alert.Description>
							You can break the relay configuration here. Use Save on the main page to validate and apply.
						</Alert.Description>
					</Alert.Root>
					{#if rawErr}
						<p class="text-sm text-destructive">{rawErr}</p>
					{/if}
					<Textarea class="min-h-[280px] font-mono text-xs leading-relaxed" bind:value={rawText} spellcheck={false} />
				</div>
				<div class="flex justify-end gap-2 border-t border-border px-6 py-4">
					<Button type="button" variant="outline" onclick={() => (rawOpen = false)}>Cancel</Button>
					<Button type="button" onclick={applyRawEditor}>Apply to form</Button>
				</div>
			</div>
		</div>
	</div>
{/if}
