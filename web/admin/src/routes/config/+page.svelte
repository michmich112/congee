<script lang="ts">
	import { onMount } from 'svelte';
	import { adminFetch } from '$lib/admin-api';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Textarea } from '$lib/components/ui/textarea';
	import * as Table from '$lib/components/ui/table';

	// Matches storage.ConfigChange JSON from GET /api/config/changelog (snake_case tags).
	type ChangelogRow = {
		created_at: number;
		summary: string;
		json_diff: string;
	};

	let rawJson = $state('');
	let changelog = $state<ChangelogRow[]>([]);
	let loadErr = $state<string | null>(null);
	let changelogErr = $state<string | null>(null);
	let saveErr = $state<string | null>(null);
	let saveOk = $state(false);
	let loading = $state(true);
	let changelogLoading = $state(false);
	let saving = $state(false);

	async function loadAll() {
		loading = true;
		loadErr = null;
		changelogErr = null;
		saveOk = false;
		try {
			const cfgRes = await adminFetch('/api/config');
			if (!cfgRes.ok) {
				loadErr = `config: HTTP ${cfgRes.status}`;
				return;
			}
			rawJson = await cfgRes.text();
		} catch (e) {
			loadErr = e instanceof Error ? e.message : 'load failed';
		} finally {
			loading = false;
		}

		// Changelog is loaded separately so a slow or stuck /api/config/changelog
		// cannot block showing the JSON editor (previously Promise.all wedged the whole page).
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
	}

	async function save() {
		saveErr = null;
		saveOk = false;
		saving = true;
		try {
			const r = await adminFetch('/api/config', {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: rawJson
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
			saveOk = true;
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
	<div class="flex flex-wrap items-end justify-between gap-4">
		<div>
			<h2 class="text-xl font-semibold tracking-tight">Configuration</h2>
			<p class="text-sm text-muted-foreground">Raw JSON file; validated on save (same rules as relay startup).</p>
		</div>
		<div class="flex gap-2">
			<Button type="button" variant="outline" disabled={loading || saving} onclick={() => void loadAll()}>Reload</Button>
			<Button type="button" disabled={loading || saving} onclick={() => void save()}>
				{saving ? 'Saving…' : 'Save'}
			</Button>
		</div>
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
	{#if saveOk}
		<p class="text-sm text-green-600 dark:text-green-400">Saved.</p>
	{/if}

	{#if loading}
		<p class="text-sm text-muted-foreground">Loading…</p>
	{:else}
		<Textarea
			class="min-h-[320px] font-mono text-xs leading-relaxed"
			bind:value={rawJson}
			spellcheck={false}
		/>

		<Card.Root>
			<Card.Header>
				<Card.Title class="text-base">Config changelog</Card.Title>
				<Card.Description>Recent writes from the admin API (newest first).</Card.Description>
			</Card.Header>
			<Card.Content class="overflow-x-auto p-0 sm:p-0">
				{#if changelogLoading}
					<p class="px-6 py-4 text-sm text-muted-foreground">Loading changelog…</p>
				{:else}
					<Table.Root>
						<Table.Header>
							<Table.Row>
								<Table.Head class="whitespace-nowrap">Created (unix)</Table.Head>
								<Table.Head>Summary</Table.Head>
								<Table.Head>Payload / diff</Table.Head>
							</Table.Row>
						</Table.Header>
						<Table.Body>
							{#each changelog as row, i (`${row.created_at}-${i}`)}
								<Table.Row>
									<Table.Cell class="font-mono text-xs tabular-nums">{row.created_at}</Table.Cell>
									<Table.Cell class="text-sm">{row.summary}</Table.Cell>
									<Table.Cell class="max-w-lg whitespace-pre-wrap break-all font-mono text-xs text-muted-foreground"
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
				{/if}
			</Card.Content>
		</Card.Root>
	{/if}
</div>
