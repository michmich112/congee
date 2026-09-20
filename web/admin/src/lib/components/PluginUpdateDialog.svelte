<script lang="ts">
	import Loader2Icon from '@lucide/svelte/icons/loader-2';
	import { adminFetch } from '$lib/admin-api';
	import {
		checkGitHubPluginUpdate,
		parseGitHubReleaseDownload,
		type PluginUpdateTarget
	} from '$lib/plugin-github';
	import { Button } from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';

	let {
		open = $bindable(false),
		plugin,
		onUpdated
	}: {
		open: boolean;
		plugin: PluginUpdateTarget | null;
		onUpdated: () => void | Promise<void>;
	} = $props();

	let url = $state('');
	let sha256 = $state('');
	let enable = $state(true);
	let busy = $state(false);
	let checkBusy = $state(false);
	let err = $state<string | null>(null);
	let hint = $state<string | null>(null);

	const canCheckGitHub = $derived(!!parseGitHubReleaseDownload(url || plugin?.source_url || ''));

	function resetFromPlugin(p: PluginUpdateTarget | null) {
		url = p?.source_url || '';
		sha256 = '';
		enable = p?.enabled ?? true;
		err = null;
		hint = null;
	}

	async function readApiError(res: Response): Promise<string> {
		try {
			const j = (await res.json()) as { error?: string };
			if (typeof j.error === 'string' && j.error) return j.error;
		} catch {
			// ignore non-JSON
		}
		return res.status === 401 ? 'Unauthorized' : `HTTP ${res.status}`;
	}

	async function checkGitHub() {
		const source = url.trim() || plugin?.source_url || '';
		checkBusy = true;
		err = null;
		hint = null;
		try {
			const result = await checkGitHubPluginUpdate(source, plugin?.version || '');
			if (result.kind === 'not_github') {
				hint = 'This source is not a GitHub release download URL, so Congee cannot look up a newer tag.';
				return;
			}
			if (result.kind === 'error') {
				err = result.error;
				return;
			}
			if (result.url) url = result.url;
			if (result.sha256) sha256 = result.sha256;
			if (!result.newer) {
				hint = result.tag
					? `GitHub latest is ${result.tag}, which matches the installed version.`
					: 'No newer GitHub release was found.';
				return;
			}
			hint = result.sha256
				? `Newer GitHub release ${result.tag} (${result.assetName || 'archive'}). SHA-256 filled from the asset digest.`
				: `Newer GitHub release ${result.tag}. Paste the archive SHA-256 from the release notes.`;
		} catch (e) {
			err = e instanceof Error ? e.message : 'GitHub check failed';
		} finally {
			checkBusy = false;
		}
	}

	async function submit(e: Event) {
		e.preventDefault();
		if (!plugin) return;
		const nextURL = url.trim();
		const nextSHA = sha256.trim();
		if (!nextURL) {
			err = 'Provide the archive URL.';
			return;
		}
		if (!nextSHA) {
			err = 'SHA-256 of the archive is required.';
			return;
		}
		busy = true;
		err = null;
		try {
			const res = await adminFetch('/api/plugins/install', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ url: nextURL, sha256: nextSHA, enable })
			});
			if (!res.ok) {
				err = await readApiError(res);
				return;
			}
			open = false;
			await onUpdated();
		} catch (e) {
			err = e instanceof Error ? e.message : 'request failed';
		} finally {
			busy = false;
		}
	}
</script>

<Dialog.Root
	bind:open
	onOpenChange={(next) => {
		if (next) resetFromPlugin(plugin);
	}}
>
	<Dialog.Content class="sm:max-w-md">
		<Dialog.Header>
			<Dialog.Title>Update plugin</Dialog.Title>
			<Dialog.Description>
				Re-installs {plugin?.name || plugin?.id || 'this plugin'} in place. Package files are replaced;
				data/ (index, secrets, models) is kept. The update hook runs schema migrations; launch then
				starts the new binary.
			</Dialog.Description>
		</Dialog.Header>
		<form class="space-y-4" onsubmit={submit}>
			<div class="space-y-2">
				<Label for="plugin-update-url">Archive URL</Label>
				<Input
					id="plugin-update-url"
					type="url"
					placeholder="https://github.com/org/repo/releases/download/v0.1.5/plugin.tar.gz"
					bind:value={url}
					disabled={busy}
				/>
			</div>
			<div class="space-y-2">
				<Label for="plugin-update-sha">SHA-256</Label>
				<Input
					id="plugin-update-sha"
					class="font-mono"
					placeholder="archive hex digest"
					autocomplete="off"
					spellcheck={false}
					bind:value={sha256}
					disabled={busy}
				/>
			</div>
			<div class="flex items-center gap-2">
				<Switch id="plugin-update-enable" bind:checked={enable} disabled={busy} />
				<Label for="plugin-update-enable">Enable after update</Label>
			</div>
			{#if hint}
				<p class="text-muted-foreground text-sm">{hint}</p>
			{/if}
			{#if err}
				<p class="text-destructive text-sm">{err}</p>
			{/if}
			<Dialog.Footer class="flex-col gap-2 sm:flex-row sm:justify-end">
				{#if canCheckGitHub}
					<Button type="button" variant="outline" disabled={busy || checkBusy} onclick={() => void checkGitHub()}>
						{#if checkBusy}
							<Loader2Icon class="size-3.5 animate-spin" aria-hidden="true" />
							Checking…
						{:else}
							Check GitHub
						{/if}
					</Button>
				{/if}
				<Button type="button" variant="outline" disabled={busy} onclick={() => (open = false)}>
					Cancel
				</Button>
				<Button type="submit" disabled={busy || !url.trim() || !sha256.trim()}>
					{#if busy}
						<Loader2Icon class="size-3.5 animate-spin" aria-hidden="true" />
						Updating…
					{:else}
						Update
					{/if}
				</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>
