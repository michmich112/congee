<script lang="ts">
	import { adminFetch } from '$lib/admin-api';
	import type { Nip11ImageSource } from '$lib/app-config';
	import { getAdminConfig } from '$lib/config/admin-config-context';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';

	let { kind }: { kind: 'icon' | 'banner' } = $props();

	const ctx = getAdminConfig();

	const choices: { value: Nip11ImageSource; label: string }[] = [
		{ value: 'default', label: 'Congee default' },
		{ value: 'upload', label: 'Upload' },
		{ value: 'url', label: 'External URL' }
	];

	function draft() {
		return ctx.draft!;
	}

	let uploadErr = $state('');
	let uploadMsg = $state('');
	let uploading = $state(false);
	let previewGen = $state(0);

	const title = $derived(kind === 'icon' ? 'Icon' : 'Banner');
	const maxLabel = $derived(kind === 'icon' ? '512 KiB' : '2 MiB');
	const source = $derived(kind === 'icon' ? draft().nip11.icon_source : draft().nip11.banner_source);
	const external = $derived(kind === 'icon' ? (draft().nip11.icon ?? '') : (draft().nip11.banner ?? ''));

	async function loadHostedPreview(
		currentKind: 'icon' | 'banner',
		currentSource: Nip11ImageSource,
		gen: number
	): Promise<string> {
		const res = await adminFetch(
			`/api/relay-assets/${currentKind}?source=${currentSource}&v=${gen}`
		);
		if (!res.ok) {
			throw new Error(
				currentSource === 'upload'
					? 'No uploaded image yet. Choose a file to store one.'
					: 'Preview is unavailable.'
			);
		}
		const blob = await res.blob();
		return URL.createObjectURL(blob);
	}

	function releasePreview(url: string) {
		return () => URL.revokeObjectURL(url);
	}

	function choose(next: Nip11ImageSource) {
		if (kind === 'icon') {
			draft().nip11.icon_source = next;
			if (next !== 'url') draft().nip11.icon = '';
		} else {
			draft().nip11.banner_source = next;
			if (next !== 'url') draft().nip11.banner = '';
		}
		uploadErr = '';
		uploadMsg = '';
		ctx.markDirty();
	}

	function setExternal(value: string) {
		if (kind === 'icon') draft().nip11.icon = value;
		else draft().nip11.banner = value;
		ctx.markDirty();
	}

	async function onFile(event: Event) {
		const input = event.currentTarget as HTMLInputElement;
		const file = input.files?.[0];
		input.value = '';
		if (!file) return;
		uploading = true;
		uploadErr = '';
		uploadMsg = '';
		try {
			const body = new FormData();
			body.set('file', file);
			const res = await adminFetch(`/api/relay-assets/${kind}`, { method: 'POST', body });
			const text = await res.text();
			if (!res.ok) {
				try {
					const j = JSON.parse(text) as { error?: string };
					uploadErr = j.error ?? text;
				} catch {
					uploadErr = text || `HTTP ${res.status}`;
				}
				return;
			}
			if (kind === 'icon') {
				draft().nip11.icon_source = 'upload';
				draft().nip11.icon = '';
			} else {
				draft().nip11.banner_source = 'upload';
				draft().nip11.banner = '';
			}
			previewGen += 1;
			let restarting = false;
			try {
				const j = JSON.parse(text) as { restarting?: boolean };
				restarting = j.restarting === true;
			} catch {
				restarting = false;
			}
			uploadMsg = restarting
				? 'Image stored. The relay is restarting to publish it.'
				: 'Image stored.';
		} catch (err) {
			uploadErr = err instanceof Error ? err.message : 'upload failed';
		} finally {
			uploading = false;
		}
	}
</script>

<div class="space-y-3">
	<p class="text-sm font-medium">{title}</p>
	<div class="flex flex-wrap gap-2" role="radiogroup" aria-label="{title} source">
		{#each choices as opt (opt.value)}
			<label
				class="inline-flex cursor-pointer items-center gap-2 rounded-md border px-3 py-1.5 text-sm {source ===
				opt.value
					? 'border-foreground bg-muted'
					: 'border-border'}"
			>
				<input
					type="radio"
					name="nip11-{kind}-source"
					value={opt.value}
					checked={source === opt.value}
					onchange={() => choose(opt.value)}
				/>
				{opt.label}
			</label>
		{/each}
	</div>
	{#if source === 'url'}
		{#if external}
			<img
				src={external}
				alt="{title} preview"
				class={kind === 'icon'
					? 'size-16 rounded-md border border-border bg-muted object-contain'
					: 'h-24 w-full max-w-md rounded-md border border-border bg-muted object-cover'}
			/>
		{/if}
	{:else}
		{#key `${kind}:${source}:${previewGen}`}
			{#await loadHostedPreview(kind, source, previewGen)}
				<p class="text-xs text-muted-foreground">Loading preview…</p>
			{:then url}
				<img
					src={url}
					alt="{title} preview"
					class={kind === 'icon'
						? 'size-16 rounded-md border border-border bg-muted object-contain'
						: 'h-24 w-full max-w-md rounded-md border border-border bg-muted object-cover'}
					{@attach () => releasePreview(url)}
				/>
			{:catch error}
				<p class="text-xs text-muted-foreground">
					{error instanceof Error ? error.message : 'Preview is unavailable.'}
				</p>
			{/await}
		{/key}
	{/if}
	{#if source === 'default'}
		<p class="text-xs text-muted-foreground">
			Built-in Congee artwork, served at <code class="rounded bg-muted px-1">/assets/{kind}</code>. Save the
			configuration to switch to this image and remove a previous upload.
		</p>
	{:else if source === 'upload'}
		<input
			type="file"
			accept="image/png,image/jpeg,image/webp,.png,.jpg,.jpeg,.webp"
			disabled={uploading}
			class="block w-full text-sm"
			onchange={(e) => void onFile(e)}
		/>
		<p class="text-xs text-muted-foreground">
			PNG, JPEG, or WebP up to {maxLabel}. A successful upload is stored beside the config file and served at
			<code class="rounded bg-muted px-1">/assets/{kind}</code>. Save other edits first if they should be written
			in the same step.
		</p>
	{:else}
		<div class="space-y-2">
			<Label for="n11-{kind}-url">External URL</Label>
			<Input
				id="n11-{kind}-url"
				type="url"
				value={external}
				oninput={(e) => setExternal(e.currentTarget.value)}
			/>
			<p class="text-xs text-muted-foreground">
				Absolute http or https URL. The relay publishes this address and does not serve
				<code class="rounded bg-muted px-1">/assets/{kind}</code>.
			</p>
		</div>
	{/if}
	{#if uploadErr}
		<p class="text-sm text-destructive">{uploadErr}</p>
	{/if}
	{#if uploadMsg}
		<p class="text-sm text-green-600 dark:text-green-400">{uploadMsg}</p>
	{/if}
</div>
