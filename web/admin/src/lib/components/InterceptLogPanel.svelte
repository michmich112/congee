<script lang="ts">
	import { adminFetch } from '$lib/admin-api';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as Table from '$lib/components/ui/table';

	type InterceptLogEntry = {
		unix_milli: number;
		duration_ms: number;
		sub_id: string;
		filters?: unknown;
		action: string;
		event_ids?: string[];
		reshape_filters?: unknown;
		subscription_filters?: unknown;
		error?: string;
	};

	type InterceptLogResponse = {
		limit?: number;
		dropped?: number;
		entries?: InterceptLogEntry[];
	};

	let { pluginId }: { pluginId: string } = $props();

	let open = $state(true);
	let loading = $state(false);
	let saving = $state(false);
	let err = $state<string | null>(null);
	let dropped = $state(0);
	let limitInput = $state(100);
	let entries = $state.raw<InterceptLogEntry[]>([]);
	let detailKey = $state<string | null>(null);

	function entryKey(e: InterceptLogEntry, i: number): string {
		return `${e.unix_milli}:${e.sub_id}:${e.action}:${i}`;
	}

	function actionVariant(action: string): 'default' | 'secondary' | 'outline' {
		if (action === 'respond') return 'default';
		if (action === 'reshape_req') return 'secondary';
		return 'outline';
	}

	function formatTime(ms: number): string {
		if (!ms) return '—';
		return new Date(ms).toLocaleString();
	}

	function prettyJSON(v: unknown): string {
		if (v === undefined || v === null) return '';
		try {
			return JSON.stringify(v, null, 2);
		} catch {
			return String(v);
		}
	}

	async function readApiError(res: Response): Promise<string> {
		try {
			const j = (await res.json()) as { error?: string };
			if (typeof j.error === 'string' && j.error) return j.error;
		} catch {
			// ignore
		}
		return `HTTP ${res.status}`;
	}

	async function loadLog(id: string, cancelled?: () => boolean, quiet = false) {
		if (!quiet) loading = true;
		err = null;
		try {
			const res = await adminFetch(`/api/plugins/${id}/intercept-log`);
			if (cancelled?.()) return;
			if (!res.ok) {
				err = await readApiError(res);
				return;
			}
			const data = (await res.json()) as InterceptLogResponse;
			if (cancelled?.()) return;
			entries = data.entries ?? [];
			dropped = data.dropped ?? 0;
			if (typeof data.limit === 'number') limitInput = data.limit;
		} catch (e) {
			if (cancelled?.()) return;
			err = e instanceof Error ? e.message : 'request failed';
		} finally {
			if (!cancelled?.()) loading = false;
		}
	}

	async function saveLimit() {
		if (!pluginId) return;
		saving = true;
		err = null;
		try {
			const res = await adminFetch(`/api/plugins/${pluginId}/intercept-log`, {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ limit: Number(limitInput) })
			});
			if (!res.ok) {
				err = await readApiError(res);
				return;
			}
			const data = (await res.json()) as { limit?: number };
			if (typeof data.limit === 'number') limitInput = data.limit;
			await loadLog(pluginId);
		} catch (e) {
			err = e instanceof Error ? e.message : 'request failed';
		} finally {
			saving = false;
		}
	}

	$effect(() => {
		const id = pluginId;
		if (!id || !open) return;
		let cancelled = false;
		void loadLog(id, () => cancelled);
		return () => {
			cancelled = true;
		};
	});
</script>

<div class="border-border shrink-0 border-b px-4 py-3 md:px-6">
	<div class="flex flex-wrap items-center justify-between gap-2">
		<div>
			<p class="text-sm font-medium">Intercept log</p>
			<p class="text-muted-foreground text-xs">
				Last {limitInput} REQ intercepts (host-side, async). Does not block the relay.
			</p>
		</div>
		<Button variant="outline" size="sm" onclick={() => (open = !open)}>
			{open ? 'Hide' : 'Show'}
		</Button>
	</div>

	{#if open}
		<div class="mt-3 flex flex-wrap items-end gap-3">
			<div class="space-y-1">
				<Label for="intercept-log-limit" class="text-xs">Rolling window</Label>
				<Input
					id="intercept-log-limit"
					class="h-7 w-24"
					type="number"
					min="0"
					max="10000"
					bind:value={limitInput}
				/>
			</div>
			<Button size="sm" disabled={saving} onclick={() => void saveLimit()}>Save</Button>
			<Button variant="outline" size="sm" disabled={loading} onclick={() => void loadLog(pluginId)}>
				Refresh
			</Button>
			{#if dropped > 0}
				<p class="text-muted-foreground text-xs">queue drops: {dropped}</p>
			{/if}
		</div>
		{#if err}
			<p class="text-destructive mt-2 text-sm">{err}</p>
		{/if}
		<div class="mt-3 max-h-72 overflow-auto rounded-md border">
			<Table.Root>
				<Table.Header>
					<Table.Row>
						<Table.Head>Time</Table.Head>
						<Table.Head>Sub</Table.Head>
						<Table.Head>Action</Table.Head>
						<Table.Head>ms</Table.Head>
						<Table.Head>Detail</Table.Head>
					</Table.Row>
				</Table.Header>
				<Table.Body>
					{#if entries.length === 0}
						<Table.Row>
							<Table.Cell colspan={5} class="text-muted-foreground text-sm">
								{loading ? 'Loading…' : 'No intercepts recorded yet'}
							</Table.Cell>
						</Table.Row>
					{:else}
						{#each entries as e, i (entryKey(e, i))}
							{@const key = entryKey(e, i)}
							<Table.Row>
								<Table.Cell class="whitespace-nowrap text-xs">{formatTime(e.unix_milli)}</Table.Cell>
								<Table.Cell class="font-mono text-xs">{e.sub_id || '—'}</Table.Cell>
								<Table.Cell>
									<Badge variant={actionVariant(e.action)}>{e.action}</Badge>
									{#if e.error}
										<span class="text-destructive ml-1 text-xs">{e.error}</span>
									{/if}
								</Table.Cell>
								<Table.Cell class="text-xs">{e.duration_ms}</Table.Cell>
								<Table.Cell>
									<Button
										variant="ghost"
										size="xs"
										onclick={() => (detailKey = detailKey === key ? null : key)}
									>
										{detailKey === key ? 'Hide' : 'View'}
									</Button>
								</Table.Cell>
							</Table.Row>
							{#if detailKey === key}
								<Table.Row>
									<Table.Cell colspan={5} class="bg-muted/40 whitespace-normal">
										<div class="grid gap-2 text-xs md:grid-cols-2">
											<div>
												<p class="mb-1 font-medium">Request</p>
												<pre class="overflow-auto">{prettyJSON({
													sub_id: e.sub_id,
													filters: e.filters
												})}</pre>
											</div>
											<div>
												<p class="mb-1 font-medium">Decision / response</p>
												<pre class="overflow-auto">{prettyJSON({
													action: e.action,
													duration_ms: e.duration_ms,
													error: e.error,
													event_ids: e.event_ids,
													reshape_filters: e.reshape_filters,
													subscription_filters: e.subscription_filters
												})}</pre>
											</div>
										</div>
									</Table.Cell>
								</Table.Row>
							{/if}
						{/each}
					{/if}
				</Table.Body>
			</Table.Root>
		</div>
	{/if}
</div>
