<script lang="ts">
	import { adminFetch } from '$lib/admin-api';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';

	type Endpoint = {
		type: 'sqlite' | 'postgres' | '';
		dsn: string;
	};

	let source = $state<Endpoint>({ type: 'sqlite', dsn: './congee.db' });
	let target = $state<Endpoint>({ type: 'postgres', dsn: '' });
	let busy = $state(false);
	let err = $state<string | null>(null);
	let done = $state(false);
	let progressPct = $state(0);
	let progressMsg = $state('');

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

	async function startMigration() {
		err = null;
		done = false;
		progressPct = 0;
		progressMsg = '';
		if (!source.type || !target.type) {
			err = 'Pick source and target types.';
			return;
		}
		if (!source.dsn.trim() || !target.dsn.trim()) {
			err = 'Source and target DSN/path are required.';
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
				err = 'Another migration is already running.';
				return;
			}
			if (!res.ok || !res.body) {
				err = `HTTP ${res.status}`;
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
							err = j.message ?? e.data;
						} catch {
							err = e.data;
						}
					}
					if (e.event === 'done') {
						done = true;
						progressPct = 100;
					}
				}
				if (ended) break;
			}
		} catch (e) {
			err = e instanceof Error ? e.message : 'request failed';
		} finally {
			busy = false;
		}
	}
</script>

<div class="space-y-8">
	<div>
		<h2 class="text-lg font-semibold">Database migration</h2>
		<p class="mt-1 text-sm text-muted-foreground">
			Copy events, tags, audit log, and config changelog between SQLite files and PostgreSQL. Target must be
			empty or you will see primary-key errors. Uses server-side paths/DSNs (not your browser filesystem).
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

			{#if err}
				<p class="text-sm text-destructive">{err}</p>
			{/if}
			{#if done && !err}
				<p class="text-sm text-muted-foreground">Migration finished.</p>
			{/if}

			<Button type="button" disabled={busy} onclick={() => void startMigration()}>
				{busy ? 'Running…' : 'Start migration'}
			</Button>
		</Card.Content>
	</Card.Root>
</div>
