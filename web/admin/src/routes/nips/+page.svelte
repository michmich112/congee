<script lang="ts">
	import { onMount } from 'svelte';
	import { adminFetch } from '$lib/admin-api';
	import * as Alert from '$lib/components/ui/alert';
	import { Badge } from '$lib/components/ui/badge';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';

	type NipRow = {
		number: number;
		title: string;
		github_url: string;
		mandatory: boolean;
		enabled: boolean;
	};

	let nips = $state<NipRow[]>([]);
	let err = $state<string | null>(null);
	let loading = $state(true);
	let busyNip = $state<number | null>(null);
	let restartRequired = $state(false);
	/** Bumps after each fetch so Switch controls remount with server `enabled` (avoids stuck UI after failed PATCH). */
	let listSyncKey = $state(0);

	async function loadNips() {
		loading = true;
		err = null;
		try {
			const r = await adminFetch('/api/nips');
			if (!r.ok) {
				err = `HTTP ${r.status}`;
				nips = [];
				return;
			}
			const data = (await r.json()) as { nips?: NipRow[] };
			nips = data.nips ?? [];
			listSyncKey++;
		} catch (e) {
			err = e instanceof Error ? e.message : 'request failed';
			nips = [];
		} finally {
			loading = false;
		}
	}

	async function setEnabled(nip: number, enabled: boolean) {
		busyNip = nip;
		err = null;
		try {
			const r = await adminFetch('/api/nips', {
				method: 'PATCH',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ nip, enabled })
			});
			const text = await r.text();
			if (!r.ok) {
				try {
					const j = JSON.parse(text) as { error?: string };
					err = j.error ?? text;
				} catch {
					err = text || `HTTP ${r.status}`;
				}
				await loadNips();
				return;
			}
			const j = JSON.parse(text) as { restart_required?: boolean };
			if (j.restart_required) restartRequired = true;
			await loadNips();
		} catch (e) {
			err = e instanceof Error ? e.message : 'request failed';
			await loadNips();
		} finally {
			busyNip = null;
		}
	}

	onMount(() => {
		void loadNips();
	});
</script>

<div class="space-y-6">
	<div>
		<h2 class="text-xl font-semibold tracking-tight">NIPs</h2>
		<p class="text-sm text-muted-foreground">Optional NIPs can be toggled in config; relay restart is required to apply.</p>
	</div>

	{#if restartRequired}
		<Alert.Root variant="default" class="border-amber-500/50 bg-amber-500/10">
			<Alert.Title>Restart required</Alert.Title>
			<Alert.Description>
				Config was updated. Restart the Congee process for NIP registration to match the file.
			</Alert.Description>
		</Alert.Root>
	{/if}

	{#if err}
		<p class="text-sm text-destructive">{err}</p>
	{/if}

	{#if loading}
		<p class="text-sm text-muted-foreground">Loading…</p>
	{:else}
		<ul class="divide-y divide-border rounded-lg border border-border">
			{#each nips as nip (nip.number)}
				<li class="flex flex-wrap items-center justify-between gap-4 px-4 py-3">
					<div class="min-w-0 flex-1 space-y-1">
						<div class="flex flex-wrap items-center gap-2">
							<span class="font-mono text-sm font-medium">NIP-{nip.number}</span>
							{#if nip.mandatory}
								<Badge variant="secondary">mandatory</Badge>
							{/if}
							{#if nip.mandatory}
								<Badge variant="secondary">mandatory</Badge>
							{:else if nip.enabled}
								<Badge>in config</Badge>
							{:else}
								<Badge variant="outline">not in config</Badge>
							{/if}
						</div>
						<p class="text-sm text-muted-foreground">{nip.title}</p>
						{#if nip.github_url}
							<a
								href={nip.github_url}
								class="text-xs text-primary underline-offset-4 hover:underline"
								target="_blank"
								rel="noreferrer">spec</a
							>
						{/if}
					</div>
					<div class="flex items-center gap-3">
						<Label class="text-muted-foreground" for="nip-{nip.number}">
							{nip.mandatory ? 'Always on' : 'Include in nips.enabled'}
						</Label>
						{#key `${listSyncKey}-${nip.number}`}
							<Switch
								id="nip-{nip.number}"
								checked={nip.enabled}
								disabled={nip.mandatory || busyNip === nip.number}
								onCheckedChange={(on: boolean) => {
									if (nip.mandatory) return;
									void setEnabled(nip.number, on);
								}}
							/>
						{/key}
					</div>
				</li>
			{/each}
		</ul>
	{/if}
</div>
