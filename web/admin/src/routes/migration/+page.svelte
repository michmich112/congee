<script lang="ts">
	import { adminFetch } from '$lib/admin-api';
	import * as Alert from '$lib/components/ui/alert';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';

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
				config_updated: boolean;
				restart_required: boolean;
				restarting: boolean;
				config_error?: string;
				target_type: string;
				target_hint: string;
		  }
		| { ok: false; message: string };

	let source = $state<Endpoint>({ type: 'sqlite', dsn: './congee.db' });
	let target = $state<Endpoint>({ type: 'postgres', dsn: '' });
	let busy = $state(false);
	let progressPct = $state(0);
	let progressMsg = $state('');
	let outcome = $state<MigrationOutcome | null>(null);

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

	async function startMigration() {
		progressPct = 0;
		progressMsg = '';
		outcome = null;
		if (!source.type || !target.type) {
			setFailed('Pick source and target types.');
			return;
		}
		if (!source.dsn.trim() || !target.dsn.trim()) {
			setFailed('Source and target DSN/path are required.');
			return;
		}
		busy = true;
		try {
			const res = await adminFetch('/api/migration/start', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					source: { type: source.type, dsn: source.dsn.trim() },
					target: { type: target.type, dsn: target.dsn.trim() }
				})
			});
			if (res.status === 409) {
				setFailed('Another migration is already running.');
				return;
			}
			if (!res.ok || !res.body) {
				setFailed(`HTTP ${res.status}`);
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
		} catch (e) {
			setFailed(e instanceof Error ? e.message : 'request failed');
		} finally {
			busy = false;
		}
	}

	function fmtCounts(c: MigrationCounts): string {
		return `${c.events} events · ${c.tags} tags · ${c.audit} audit · ${c.changelog} changelog`;
	}
</script>

<div class="space-y-8">
	<div>
		<h2 class="text-lg font-semibold">Database migration</h2>
		<p class="mt-1 text-sm text-muted-foreground">
			Copy events, tags, audit log, and config changelog between SQLite files and PostgreSQL. Target must be
			empty or you will see primary-key errors. Uses server-side paths/DSNs (not your browser filesystem). After
			a successful copy, the relay config file is updated to use the target database; restart the process to run
			on the new store.
		</p>
	</div>

	<Card.Root>
		<Card.Header>
			<Card.Title>Endpoints</Card.Title>
			<Card.Description>JSON body mirrors <code class="text-xs">POST /api/migration/start</code>.</Card.Description>
		</Card.Header>
		<Card.Content class="space-y-6">
			<div class="grid gap-6 sm:grid-cols-2">
				<div class="space-y-3">
					<p class="text-sm font-medium">Source</p>
					<div class="space-y-2">
						<Label for="src-type">Type</Label>
						<select
							id="src-type"
							class="flex h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
							bind:value={source.type}
						>
							<option value="sqlite">sqlite</option>
							<option value="postgres">postgres</option>
						</select>
					</div>
					<div class="space-y-2">
						<Label for="src-dsn">DSN or path</Label>
						<Input id="src-dsn" bind:value={source.dsn} placeholder="./congee.db or postgres://..." />
					</div>
				</div>
				<div class="space-y-3">
					<p class="text-sm font-medium">Target</p>
					<div class="space-y-2">
						<Label for="dst-type">Type</Label>
						<select
							id="dst-type"
							class="flex h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
							bind:value={target.type}
						>
							<option value="sqlite">sqlite</option>
							<option value="postgres">postgres</option>
						</select>
					</div>
					<div class="space-y-2">
						<Label for="dst-dsn">DSN or path</Label>
						<Input id="dst-dsn" bind:value={target.dsn} placeholder="postgres://... or ./new.db" />
					</div>
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
					{:else if outcome.config_error}
						<Alert.Root variant="destructive">
							<Alert.Title>Config file not updated</Alert.Title>
							<Alert.Description>{outcome.config_error}</Alert.Description>
						</Alert.Root>
					{/if}
				</div>
			{/if}

			<Button type="button" disabled={busy} onclick={() => void startMigration()}>
				{busy ? 'Running…' : 'Start migration'}
			</Button>
		</Card.Content>
	</Card.Root>
</div>
