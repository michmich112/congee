<script lang="ts">
	import Blocks from '@lucide/svelte/icons/blocks';
	import { onMount } from 'svelte';
	import { adminFetch } from '$lib/admin-api';
	import AdminPageHeading from '$lib/components/AdminPageHeading.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import * as Table from '$lib/components/ui/table';
	import PluginActionsMenu from '$lib/components/PluginActionsMenu.svelte';
	import { pluginNav } from '$lib/plugin-nav.svelte';

	type PluginRow = {
		id: string;
		name: string;
		version: string;
		enabled: boolean;
		state: string;
		ready?: boolean;
		last_error?: string;
	};

	let plugins = $state<PluginRow[]>([]);
	let loading = $state(true);
	let err = $state<string | null>(null);
	let actionBusy = $state<string | null>(null);

	let installOpen = $state(false);
	let installUrl = $state('');
	let installSha256 = $state('');
	let installPath = $state('');
	let installEnable = $state(true);
	let installBusy = $state(false);
	let installErr = $state<string | null>(null);

	async function readApiError(res: Response): Promise<string> {
		try {
			const j = (await res.json()) as { error?: string };
			if (typeof j.error === 'string' && j.error) return j.error;
		} catch {
			// ignore non-JSON
		}
		return res.status === 401 ? 'Unauthorized' : `HTTP ${res.status}`;
	}

	async function loadPlugins() {
		loading = true;
		err = null;
		try {
			const res = await adminFetch('/api/plugins');
			if (!res.ok) {
				err = await readApiError(res);
				plugins = [];
				return;
			}
			const data = (await res.json()) as { plugins?: PluginRow[] };
			plugins = data.plugins ?? [];
			pluginNav.items = plugins.map((p) => ({
				id: p.id,
				name: p.name,
				enabled: p.enabled,
				state: p.state
			}));
		} catch (e) {
			err = e instanceof Error ? e.message : 'request failed';
			plugins = [];
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		void loadPlugins();
	});

	function resetInstallForm() {
		installUrl = '';
		installSha256 = '';
		installPath = '';
		installEnable = true;
		installErr = null;
	}

	async function installPlugin(e: Event) {
		e.preventDefault();
		const path = installPath.trim();
		const url = installUrl.trim();
		const sha256 = installSha256.trim();
		if (!path && !url) {
			installErr = 'Provide a URL (and optional sha256) or a local path.';
			return;
		}
		installBusy = true;
		installErr = null;
		try {
			const body = path
				? { path, enable: installEnable }
				: { url, sha256, enable: installEnable };
			const res = await adminFetch('/api/plugins/install', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(body)
			});
			if (!res.ok) {
				installErr = await readApiError(res);
				return;
			}
			installOpen = false;
			resetInstallForm();
			await loadPlugins();
		} catch (e) {
			installErr = e instanceof Error ? e.message : 'request failed';
		} finally {
			installBusy = false;
		}
	}

	async function postPluginAction(id: string, action: 'enable' | 'disable' | 'uninstall') {
		const key = `${id}:${action}`;
		actionBusy = key;
		err = null;
		try {
			const res = await adminFetch(`/api/plugins/${id}/${action}`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: action === 'uninstall' ? JSON.stringify({ wipe_data: false }) : undefined
			});
			if (!res.ok) {
				err = await readApiError(res);
				return;
			}
			await loadPlugins();
		} catch (e) {
			err = e instanceof Error ? e.message : 'request failed';
		} finally {
			actionBusy = null;
		}
	}

	function uninstallPlugin(p: PluginRow) {
		if (!confirm(`Uninstall plugin “${p.name || p.id}”?`)) return;
		void postPluginAction(p.id, 'uninstall');
	}

	function stateBadgeVariant(state: string): 'default' | 'secondary' | 'destructive' | 'outline' {
		if (state === 'ready') return 'default';
		if (state === 'degraded') return 'destructive';
		if (state === 'starting') return 'secondary';
		return 'outline';
	}
</script>

<div class="space-y-8">
	<AdminPageHeading
		title="Plugins"
		subtitle="Install, enable, and manage relay plugins."
		Icon={Blocks}
	/>

	{#if err}
		<p class="text-destructive text-sm">{err}</p>
	{/if}

	<Card.Root>
		<Card.Header>
			<Card.Title>Installed plugins</Card.Title>
			<Card.Description>Each plugin runs in its own process and can expose a hosted admin UI.</Card.Description>
			<Card.Action>
				<Button type="button" onclick={() => (installOpen = true)}>Install</Button>
			</Card.Action>
		</Card.Header>
		<Card.Content class="p-0">
			{#if loading}
				<p class="text-muted-foreground px-4 py-6 text-sm">Loading…</p>
			{:else}
				<Table.Root>
					<Table.Header>
						<Table.Row>
							<Table.Head>Name</Table.Head>
							<Table.Head>ID</Table.Head>
							<Table.Head>Version</Table.Head>
							<Table.Head>State</Table.Head>
							<Table.Head>Enabled</Table.Head>
							<Table.Head class="text-right">Actions</Table.Head>
						</Table.Row>
					</Table.Header>
					<Table.Body>
						{#each plugins as p (p.id)}
							<Table.Row>
								<Table.Cell>
									<a
										href="/plugins/{p.id}"
										class="font-medium text-primary underline-offset-4 hover:underline"
									>
										{p.name || p.id}
									</a>
									{#if p.last_error}
										<p class="text-destructive mt-1 max-w-xs truncate text-xs" title={p.last_error}>
											{p.last_error}
										</p>
									{/if}
								</Table.Cell>
								<Table.Cell class="font-mono text-xs">{p.id}</Table.Cell>
								<Table.Cell class="font-mono text-xs">{p.version || '—'}</Table.Cell>
								<Table.Cell>
									<Badge variant={stateBadgeVariant(p.state)}>{p.state || '—'}</Badge>
								</Table.Cell>
								<Table.Cell>
									{#if p.enabled}
										<Badge>enabled</Badge>
									{:else}
										<Badge variant="outline">disabled</Badge>
									{/if}
								</Table.Cell>
								<Table.Cell class="text-right">
									<div class="flex justify-end">
										<PluginActionsMenu
											plugin={p}
											busy={actionBusy === `${p.id}:enable` ||
												actionBusy === `${p.id}:disable` ||
												actionBusy === `${p.id}:uninstall`}
											onEnable={() => void postPluginAction(p.id, 'enable')}
											onDisable={() => void postPluginAction(p.id, 'disable')}
											onUninstall={() => uninstallPlugin(p)}
										/>
									</div>
								</Table.Cell>
							</Table.Row>
						{:else}
							<Table.Row>
								<Table.Cell colspan={6} class="text-muted-foreground text-center text-sm">
									No plugins installed
								</Table.Cell>
							</Table.Row>
						{/each}
					</Table.Body>
				</Table.Root>
			{/if}
		</Card.Content>
	</Card.Root>
</div>

<Dialog.Root
	bind:open={installOpen}
	onOpenChange={(open) => {
		if (!open) resetInstallForm();
	}}
>
	<Dialog.Content class="sm:max-w-md">
		<Dialog.Header>
			<Dialog.Title>Install plugin</Dialog.Title>
			<Dialog.Description>
				Install from a URL with optional sha256, or from a local path that contains plugin.json.
			</Dialog.Description>
		</Dialog.Header>
		<form class="space-y-4" onsubmit={installPlugin}>
			<div class="space-y-2">
				<Label for="plugin-url">URL</Label>
				<Input
					id="plugin-url"
					type="url"
					placeholder="https://example.com/plugin.tar.gz"
					bind:value={installUrl}
					disabled={installBusy}
				/>
			</div>
			<div class="space-y-2">
				<Label for="plugin-sha">SHA-256</Label>
				<Input
					id="plugin-sha"
					class="font-mono"
					placeholder="optional hex digest"
					autocomplete="off"
					spellcheck={false}
					bind:value={installSha256}
					disabled={installBusy}
				/>
			</div>
			<div class="space-y-2">
				<Label for="plugin-path">Local path</Label>
				<Input
					id="plugin-path"
					class="font-mono"
					placeholder="/path/to/plugin"
					bind:value={installPath}
					disabled={installBusy}
				/>
				<p class="text-muted-foreground text-xs">If a local path is set, it is used instead of the URL.</p>
			</div>
			<div class="flex items-center gap-2">
				<Switch id="plugin-enable" bind:checked={installEnable} disabled={installBusy} />
				<Label for="plugin-enable">Enable after install</Label>
			</div>
			{#if installErr}
				<p class="text-destructive text-sm">{installErr}</p>
			{/if}
			<Dialog.Footer>
				<Button type="button" variant="outline" disabled={installBusy} onclick={() => (installOpen = false)}>
					Cancel
				</Button>
				<Button type="submit" disabled={installBusy || (!installUrl.trim() && !installPath.trim())}>
					{installBusy ? 'Installing…' : 'Install'}
				</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>
