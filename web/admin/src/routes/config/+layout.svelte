<script lang="ts">
	import { onMount, setContext } from 'svelte';
	import Code from '@lucide/svelte/icons/code';
	import ChevronDown from '@lucide/svelte/icons/chevron-down';
	import { adminFetch } from '$lib/admin-api';
	import { parseConfigJson, type AppConfig } from '$lib/app-config';
	import {
		ADMIN_CONFIG_CTX,
		type AdminConfigContext,
		type ChangelogRow,
		type NipRow
	} from '$lib/config/admin-config-context';
	import * as Alert from '$lib/components/ui/alert';
	import { Button } from '$lib/components/ui/button';
	import { Textarea } from '$lib/components/ui/textarea';
	import TableTimestampModeSelect from '$lib/components/TableTimestampModeSelect.svelte';
	import TimestampCell from '$lib/components/TimestampCell.svelte';
	import * as Table from '$lib/components/ui/table';

	let { children } = $props();

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
	let relayIdentity = $state<{ pubkey_hex: string; npub: string } | null>(null);

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
		relayIdentity = null;
		try {
			const idRes = await adminFetch('/api/relay-identity');
			if (idRes.ok) {
				const j = (await idRes.json()) as { pubkey_hex?: string; npub?: string };
				if (j.pubkey_hex && j.npub) {
					relayIdentity = { pubkey_hex: j.pubkey_hex, npub: j.npub };
				}
			}
		} catch {
			relayIdentity = null;
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

	const ctx: AdminConfigContext = {
		get draft() {
			return draft;
		},
		get nipCatalog() {
			return nipCatalog;
		},
		get relayIdentity() {
			return relayIdentity;
		},
		get loading() {
			return loading;
		},
		get saving() {
			return saving;
		},
		get loadErr() {
			return loadErr;
		},
		get changelogErr() {
			return changelogErr;
		},
		get saveErr() {
			return saveErr;
		},
		get saveOk() {
			return saveOk;
		},
		get saveMessage() {
			return saveMessage;
		},
		get changelogLoading() {
			return changelogLoading;
		},
		get changelog() {
			return changelog;
		},
		get changelogExpanded() {
			return changelogExpanded;
		},
		set changelogExpanded(v: boolean) {
			changelogExpanded = v;
		},
		get dirty() {
			return dirty;
		},
		markDirty,
		setNipEnabled,
		selectClass
	};

	setContext(ADMIN_CONFIG_CTX, ctx);

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
		{@render children()}

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
