<script lang="ts">
	import { onMount } from 'svelte';
	import Plus from '@lucide/svelte/icons/plus';
	import { adminFetch } from '$lib/admin-api';
	import { parseConfigJson } from '$lib/app-config';
	import * as Alert from '$lib/components/ui/alert';
	import { Button, buttonVariants } from '$lib/components/ui/button';
	import { ButtonGroup } from '$lib/components/ui/button-group';
	import * as Card from '$lib/components/ui/card';
	import * as Dialog from '$lib/components/ui/dialog';
	import {
		DropdownMenu,
		DropdownMenuContent,
		DropdownMenuGroup,
		DropdownMenuItem,
		DropdownMenuTrigger
	} from '$lib/components/ui/dropdown-menu';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { cn } from '$lib/utils';

	type Endpoint = {
		type: 'sqlite' | 'postgres' | '';
		dsn: string;
	};

	type MigrationCounts = {
		events: number;
		tags: number;
		audit: number;
		changelog: number;
	};

	type MigrationSummary = {
		source: MigrationCounts;
		destination_final: MigrationCounts;
		events_inserted: number;
		events_skipped: number;
		tags_added: number;
		audit_inserted: number;
		audit_skipped: number;
		changelog_copied: number;
	};

	type MigrationOutcome =
		| {
				ok: true;
				summary: MigrationSummary;
				make_target_primary: boolean;
				config_updated: boolean;
				restart_required: boolean;
				restarting: boolean;
				config_error?: string;
				target_type: string;
				target_hint: string;
		  }
		| { ok: false; message: string };

	type TargetPreflight = {
		status: string;
		expected_version: number;
		reported_version?: number;
		has_events_table: boolean;
		has_version_table: boolean;
		detail: string;
	};

	let source = $state<Endpoint>({ type: '', dsn: '' });
	let target = $state<Endpoint>({ type: 'postgres', dsn: '' });
	let sourceHydrated = $state(false);
	let configLoadError = $state<string | null>(null);
	let busy = $state(false);
	let progressPct = $state(0);
	let progressMsg = $state('');
	let outcome = $state<MigrationOutcome | null>(null);
	let schemaMismatchOpen = $state(false);
	let schemaMismatchBody = $state('');
	let pendingMakeTargetPrimary = $state(false);

	function canonicalDbType(t: string): 'sqlite' | 'postgres' {
		const x = (t || '').trim().toLowerCase();
		return x === 'postgres' ? 'postgres' : 'sqlite';
	}

	onMount(() => {
		void (async () => {
			try {
				const res = await adminFetch('/api/config');
				if (!res.ok) {
					configLoadError = `Could not load config (HTTP ${res.status}).`;
					return;
				}
				const cfg = parseConfigJson(await res.text());
				const dsn = (cfg.database?.dsn ?? '').trim();
				if (!dsn) {
					configLoadError = 'Config has an empty database.dsn.';
					return;
				}
				source = { type: canonicalDbType(cfg.database?.type ?? ''), dsn };
				sourceHydrated = true;
			} catch (e) {
				configLoadError = e instanceof Error ? e.message : 'Failed to load config.';
			}
		})();
	});

	function parseSSEChunk(buffer: string): { events: { event: string; data: string }[]; rest: string } {
		const events: { event: string; data: string }[] = [];
		const parts = buffer.split('\n\n');
		const rest = parts.pop() ?? '';
		for (const block of parts) {
			let ev = 'message';
			let data = '';
			for (const line of block.split('\n')) {
				if (line.startsWith('event:')) ev = line.slice(6).trim();
				else if (line.startsWith('data:')) data = line.slice(5).trim();
			}
			if (data) events.push({ event: ev, data });
		}
		return { events, rest };
	}

	function setFailed(message: string) {
		outcome = { ok: false, message };
	}

	async function runMigrationSSE(makeTargetPrimary: boolean) {
		const res = await adminFetch('/api/migration/start', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({
				source: { type: source.type, dsn: source.dsn.trim() },
				target: { type: target.type, dsn: target.dsn.trim() },
				make_target_primary: makeTargetPrimary
			})
		});
		if (res.status === 409) {
			setFailed('Another migration is already running.');
			return;
		}
		if (!res.ok || !res.body) {
			let errText = '';
			try {
				errText = (await res.text()).trim();
			} catch {
				/* ignore */
			}
			setFailed(errText ? `HTTP ${res.status}: ${errText}` : `HTTP ${res.status}`);
			return;
		}
		const reader = res.body.getReader();
		const dec = new TextDecoder();
		let buf = '';
		while (true) {
			const { value, done: ended } = await reader.read();
			if (value) buf += dec.decode(value, { stream: true });
			const { events, rest } = parseSSEChunk(buf);
			buf = rest;
			for (const e of events) {
				if (e.event === 'progress') {
					try {
						const j = JSON.parse(e.data) as { percent?: number; message?: string };
						if (typeof j.percent === 'number') progressPct = Math.min(100, Math.max(0, j.percent));
						if (typeof j.message === 'string') progressMsg = j.message;
					} catch {
						/* ignore */
					}
				}
				if (e.event === 'error') {
					try {
						const j = JSON.parse(e.data) as { message?: string };
						setFailed(j.message ?? e.data);
					} catch {
						setFailed(e.data);
					}
				}
				if (e.event === 'done') {
					progressPct = 100;
					try {
						const j = JSON.parse(e.data) as {
							summary?: MigrationSummary;
							make_target_primary?: boolean;
							config_updated?: boolean;
							restart_required?: boolean;
							restarting?: boolean;
							config_error?: string;
							target_type?: string;
							target_dsn_nonsecret?: string;
						};
						if (j.summary) {
							outcome = {
								ok: true,
								summary: j.summary,
								make_target_primary: j.make_target_primary === true,
								config_updated: j.config_updated === true,
								restart_required: j.restart_required === true,
								restarting: j.restarting === true,
								config_error: typeof j.config_error === 'string' ? j.config_error : undefined,
								target_type: typeof j.target_type === 'string' ? j.target_type : target.type,
								target_hint:
									typeof j.target_dsn_nonsecret === 'string' ? j.target_dsn_nonsecret : ''
							};
						} else {
							setFailed('Migration finished but response had no summary.');
						}
					} catch {
						setFailed('Could not parse migration result.');
					}
				}
			}
			if (ended) break;
		}
	}

	async function requestMigrationStart(makeTargetPrimary: boolean) {
		progressPct = 0;
		progressMsg = '';
		outcome = null;
		if (configLoadError || !sourceHydrated) {
			setFailed(configLoadError ?? 'Current database is still loading.');
			return;
		}
		if (!target.type) {
			setFailed('Pick a target type.');
			return;
		}
		if (!target.dsn.trim()) {
			setFailed('Target DSN or path is required.');
			return;
		}

		busy = true;
		try {
			const pfRes = await adminFetch('/api/migration/target-preflight', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					target: { type: target.type, dsn: target.dsn.trim() }
				})
			});
			if (!pfRes.ok) {
				let errText = '';
				try {
					errText = (await pfRes.text()).trim();
				} catch {
					/* ignore */
				}
				setFailed(errText ? `Preflight HTTP ${pfRes.status}: ${errText}` : `Preflight HTTP ${pfRes.status}`);
				return;
			}
			let pf: TargetPreflight;
			try {
				pf = (await pfRes.json()) as TargetPreflight;
			} catch {
				setFailed('Could not parse preflight response.');
				return;
			}

			if (pf.status === 'ahead') {
				setFailed(pf.detail || 'Target schema is newer than this relay binary.');
				return;
			}
			if (pf.status === 'unreadable') {
				setFailed(pf.detail || 'Could not read target schema.');
				return;
			}
			if (pf.status === 'behind') {
				const rep =
					typeof pf.reported_version === 'number' ? String(pf.reported_version) : 'unknown';
				const exp = String(pf.expected_version ?? '?');
				schemaMismatchBody =
					(pf.detail ? pf.detail + '\n\n' : '') +
					`Reported schema version: ${rep}. This binary expects: ${exp}.\n\n` +
					'Continuing will apply database schema migrations (DDL) on the target, then copy data.';
				pendingMakeTargetPrimary = makeTargetPrimary;
				schemaMismatchOpen = true;
				return;
			}

			await runMigrationSSE(makeTargetPrimary);
		} catch (e) {
			setFailed(e instanceof Error ? e.message : 'request failed');
		} finally {
			busy = false;
		}
	}

	function onSchemaMismatchContinue() {
		schemaMismatchOpen = false;
		const makePrimary = pendingMakeTargetPrimary;
		progressPct = 0;
		progressMsg = '';
		outcome = null;
		busy = true;
		void (async () => {
			try {
				await runMigrationSSE(makePrimary);
			} catch (e) {
				setFailed(e instanceof Error ? e.message : 'request failed');
			} finally {
				busy = false;
			}
		})();
	}

	async function startMigration(makeTargetPrimary: boolean) {
		await requestMigrationStart(makeTargetPrimary);
	}

	function fmtCounts(c: MigrationCounts): string {
		return `${c.events} events · ${c.tags} tags · ${c.audit} audit · ${c.changelog} changelog`;
	}
</script>

<Card.Root>
	<Card.Header>
		<Card.Title class="text-base">Database migration</Card.Title>
		<Card.Description>
			Copy events, tags, audit log, and config changelog to a different database. Duplicate events will be skipped.
			The source is always the database from the relay JSON config.
		</Card.Description>
	</Card.Header>
	<Card.Content class="space-y-6">
		{#if configLoadError}
			<Alert.Root variant="destructive">
				<Alert.Title>Could not load current database</Alert.Title>
				<Alert.Description>{configLoadError}</Alert.Description>
			</Alert.Root>
		{:else if !sourceHydrated}
			<p class="text-sm text-muted-foreground">Loading current database from config…</p>
		{/if}

		<div class="grid gap-8 sm:grid-cols-2 sm:gap-10">
			<div class="space-y-4">
				<h3 class="text-sm font-semibold tracking-tight">Source</h3>
				<div class="grid grid-cols-[auto_minmax(0,1fr)] items-center gap-x-3 gap-y-3">
					<Label for="src-type" class="shrink-0">Type</Label>
					<select
						id="src-type"
						class="border-input bg-muted/40 h-9 w-full min-w-0 cursor-not-allowed rounded-md border px-3 text-sm disabled:opacity-90"
						bind:value={source.type}
						disabled
					>
						<option value="sqlite">sqlite</option>
						<option value="postgres">postgres</option>
					</select>
					<Label for="src-dsn" class="shrink-0">DSN or path</Label>
					<Input
						id="src-dsn"
						bind:value={source.dsn}
						placeholder={sourceHydrated ? '' : 'Loading…'}
						readonly
						class="cursor-default bg-muted/40"
					/>
				</div>
			</div>
			<div class="space-y-4">
				<h3 class="text-sm font-semibold tracking-tight">Target</h3>
				<div class="grid grid-cols-[auto_minmax(0,1fr)] items-center gap-x-3 gap-y-3">
					<Label for="dst-type" class="shrink-0">Type</Label>
					<select
						id="dst-type"
						class="border-input bg-background h-9 w-full min-w-0 rounded-md border px-3 text-sm"
						bind:value={target.type}
					>
						<option value="sqlite">sqlite</option>
						<option value="postgres">postgres</option>
					</select>
					<Label for="dst-dsn" class="shrink-0">DSN or path</Label>
					<Input id="dst-dsn" bind:value={target.dsn} placeholder="postgres://... or ./new.db" />
				</div>
				{#if canonicalDbType(target.type) === 'postgres'}
					<Alert.Root class="border-amber-500/40 bg-amber-500/5">
						<Alert.Title>Postgres target</Alert.Title>
						<Alert.Description>
							Ensure no other Congee instance (or app using the same DSN) writes to this database until
							the migration completes. Concurrent writers can corrupt the copy.
						</Alert.Description>
					</Alert.Root>
				{/if}
			</div>
		</div>

		<div class="space-y-2">
			<div class="flex justify-between text-xs text-muted-foreground">
				<span>{progressMsg || (busy ? 'Starting…' : 'Idle')}</span>
				<span>{Math.round(progressPct)}%</span>
			</div>
			<div class="h-2 w-full overflow-hidden rounded-full bg-muted">
				<div
					class="h-full bg-primary transition-[width] duration-300 ease-out"
					style={`width: ${progressPct}%`}
				></div>
			</div>
		</div>

		{#if outcome && !outcome.ok}
			<Alert.Root variant="destructive">
				<Alert.Title>Migration did not complete</Alert.Title>
				<Alert.Description>{outcome.message}</Alert.Description>
			</Alert.Root>
		{/if}

		{#if outcome?.ok}
			<div class="space-y-4 rounded-lg border border-border bg-muted/30 p-4">
				<p class="text-sm font-medium">Migration summary</p>
				<dl class="grid gap-2 text-sm sm:grid-cols-2">
					<dt class="text-muted-foreground">Source (before)</dt>
					<dd class="font-mono text-xs">{fmtCounts(outcome.summary.source)}</dd>
					<dt class="text-muted-foreground">Destination (after)</dt>
					<dd class="font-mono text-xs">{fmtCounts(outcome.summary.destination_final)}</dd>
				</dl>
				<ul class="list-inside list-disc text-sm text-muted-foreground">
					<li>
						Events: {outcome.summary.events_inserted} inserted, {outcome.summary.events_skipped} skipped
						({outcome.summary.tags_added} tag rows on inserted events)
					</li>
					<li>
						Audit: {outcome.summary.audit_inserted} inserted, {outcome.summary.audit_skipped} skipped
					</li>
					<li>Config changelog rows copied: {outcome.summary.changelog_copied}</li>
				</ul>
				{#if outcome.config_updated}
					<p class="text-sm">
						Configuration updated to
						<span class="font-medium">{outcome.target_type}</span>
						{#if outcome.target_hint}
							<span class="text-muted-foreground"> ({outcome.target_hint})</span>
						{/if}.
					</p>
					{#if outcome.restart_required}
						<Alert.Root>
							<Alert.Title>Restart required</Alert.Title>
							<Alert.Description>
								Restart the Congee process to connect to the new database.
								{#if outcome.restarting}
									A restart was scheduled.
								{/if}
							</Alert.Description>
						</Alert.Root>
					{/if}
				{:else if outcome.make_target_primary && outcome.config_error}
					<Alert.Root variant="destructive">
						<Alert.Title>Config file not updated</Alert.Title>
						<Alert.Description>{outcome.config_error}</Alert.Description>
					</Alert.Root>
				{:else if !outcome.make_target_primary}
					<p class="text-sm text-muted-foreground">
						Relay configuration was left unchanged (data copy only). To point Congee at this target, use
						<span class="font-medium text-foreground">Start migration &amp; make target primary DB</span>
						from the <span class="font-medium text-foreground">+</span> menu or edit <code class="text-xs">database</code> in the config file.
					</p>
				{/if}
			</div>
		{/if}

		<ButtonGroup>
			<Button
				type="button"
				variant="outline"
				disabled={busy || !!configLoadError || !sourceHydrated}
				onclick={() => void startMigration(false)}
			>
				{busy ? 'Running…' : 'Start migration'}
			</Button>
			<DropdownMenu>
				<DropdownMenuTrigger
					class={cn(
						buttonVariants({ variant: 'outline', size: 'icon' }),
						'disabled:pointer-events-none disabled:opacity-50'
					)}
					disabled={busy || !!configLoadError || !sourceHydrated}
					aria-label="Make target primary database (opens menu)"
				>
					<Plus class="size-4 opacity-90" />
				</DropdownMenuTrigger>
				<DropdownMenuContent align="end" class="min-w-56">
					<DropdownMenuGroup>
						<DropdownMenuItem
							disabled={busy || !!configLoadError || !sourceHydrated}
							onclick={() => void startMigration(true)}
							class="whitespace-normal"
						>
							Start migration &amp; make target primary DB
						</DropdownMenuItem>
					</DropdownMenuGroup>
				</DropdownMenuContent>
			</DropdownMenu>
		</ButtonGroup>
	</Card.Content>
</Card.Root>

<Dialog.Root bind:open={schemaMismatchOpen}>
	<Dialog.Content class="sm:max-w-lg">
		<Dialog.Header>
			<Dialog.Title>Schema mismatch</Dialog.Title>
			<Dialog.Description class="whitespace-pre-wrap text-left">{schemaMismatchBody}</Dialog.Description>
		</Dialog.Header>
		<Dialog.Footer class="gap-2 sm:justify-end">
			<Button type="button" variant="outline" onclick={() => (schemaMismatchOpen = false)}>Cancel</Button>
			<Button type="button" onclick={onSchemaMismatchContinue}>Continue</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
