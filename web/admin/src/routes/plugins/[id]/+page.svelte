<script lang="ts">
	import Blocks from '@lucide/svelte/icons/blocks';
	import { page } from '$app/state';
	import { adminFetch } from '$lib/admin-api';
	import AdminPageHeading from '$lib/components/AdminPageHeading.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';

	type PluginRow = {
		id: string;
		name: string;
		version: string;
		enabled: boolean;
		state: string;
		ready?: boolean;
		last_error?: string;
	};

	type PluginApiMessage = {
		type: 'congee:plugin-api';
		id: unknown;
		method?: unknown;
		path?: unknown;
		body?: unknown;
	};

	const pluginId = $derived(page.params.id ?? '');

	let plugin = $state<PluginRow | null>(null);
	let loading = $state(true);
	let err = $state<string | null>(null);
	let actionBusy = $state(false);
	let iframeEl: HTMLIFrameElement | null = null;

	const iframeSrc = $derived(pluginId ? `/plugin-ui/${pluginId}/` : '');

	async function readApiError(res: Response): Promise<string> {
		try {
			const j = (await res.json()) as { error?: string };
			if (typeof j.error === 'string' && j.error) return j.error;
		} catch {
			// ignore non-JSON
		}
		return res.status === 401 ? 'Unauthorized' : `HTTP ${res.status}`;
	}

	async function loadPlugin(id: string, cancelled?: () => boolean) {
		loading = true;
		err = null;
		try {
			const res = await adminFetch(`/api/plugins/${id}`);
			if (cancelled?.()) return;
			if (!res.ok) {
				err = await readApiError(res);
				plugin = null;
				return;
			}
			const data = (await res.json()) as { plugin?: PluginRow };
			if (cancelled?.()) return;
			plugin = data.plugin ?? null;
			if (!plugin) err = 'Plugin not found';
		} catch (e) {
			if (cancelled?.()) return;
			err = e instanceof Error ? e.message : 'request failed';
			plugin = null;
		} finally {
			if (!cancelled?.()) loading = false;
		}
	}

	$effect(() => {
		const id = pluginId;
		if (!id) return;
		let cancelled = false;
		void loadPlugin(id, () => cancelled);
		return () => {
			cancelled = true;
		};
	});

	function currentTheme(): 'dark' | 'light' {
		return document.documentElement.classList.contains('dark') ? 'dark' : 'light';
	}

	function attachIframe(node: HTMLIFrameElement) {
		iframeEl = node;
		return () => {
			iframeEl = null;
		};
	}

	function postMessageTargetOrigin(origin: string): string {
		return origin === 'null' ? '*' : origin;
	}

	function onIframeLoad() {
		iframeEl?.contentWindow?.postMessage(
			{ type: 'congee:theme', theme: currentTheme() },
			{ targetOrigin: postMessageTargetOrigin('null') }
		);
	}

	function isAllowedPluginPath(path: string, id: string): boolean {
		const prefix = `/api/plugins/${id}/`;
		if (!path.startsWith(prefix)) return false;
		try {
			const resolved = new URL(path, window.location.origin).pathname;
			return resolved.startsWith(prefix);
		} catch {
			return false;
		}
	}

	function replyPluginApi(
		source: MessageEventSource | null,
		origin: string,
		payload: { type: 'congee:plugin-api-result'; id: unknown; status: number; json: unknown }
	) {
		if (!source || !('postMessage' in source)) return;
		source.postMessage(payload, { targetOrigin: postMessageTargetOrigin(origin) });
	}

	async function onPluginMessage(event: MessageEvent) {
		if (event.origin !== window.location.origin && event.origin !== 'null') return;
		if (event.source !== iframeEl?.contentWindow) return;
		const data = event.data as PluginApiMessage | null;
		if (!data || data.type !== 'congee:plugin-api') return;

		const path = typeof data.path === 'string' ? data.path : '';
		const id = pluginId;
		if (!id || !isAllowedPluginPath(path, id)) {
			replyPluginApi(event.source, event.origin, {
				type: 'congee:plugin-api-result',
				id: data.id,
				status: 403,
				json: { error: 'path not allowed' }
			});
			return;
		}

		const method =
			typeof data.method === 'string' && data.method.trim()
				? data.method.trim().toUpperCase()
				: 'GET';
		const init: RequestInit = { method };
		if (data.body !== undefined && method !== 'GET' && method !== 'HEAD') {
			init.headers = { 'Content-Type': 'application/json' };
			init.body = typeof data.body === 'string' ? data.body : JSON.stringify(data.body);
		}

		try {
			const res = await adminFetch(path, init);
			let json: unknown = null;
			try {
				json = await res.json();
			} catch {
				json = null;
			}
			replyPluginApi(event.source, event.origin, {
				type: 'congee:plugin-api-result',
				id: data.id,
				status: res.status,
				json
			});
		} catch (e) {
			replyPluginApi(event.source, event.origin, {
				type: 'congee:plugin-api-result',
				id: data.id,
				status: 0,
				json: { error: e instanceof Error ? e.message : 'request failed' }
			});
		}
	}

	async function toggleEnabled() {
		if (!plugin) return;
		const action = plugin.enabled ? 'disable' : 'enable';
		actionBusy = true;
		err = null;
		try {
			const res = await adminFetch(`/api/plugins/${plugin.id}/${action}`, { method: 'POST' });
			if (!res.ok) {
				err = await readApiError(res);
				return;
			}
			await loadPlugin(plugin.id);
		} catch (e) {
			err = e instanceof Error ? e.message : 'request failed';
		} finally {
			actionBusy = false;
		}
	}

	function stateBadgeVariant(state: string): 'default' | 'secondary' | 'destructive' | 'outline' {
		if (state === 'ready') return 'default';
		if (state === 'degraded') return 'destructive';
		if (state === 'starting') return 'secondary';
		return 'outline';
	}
</script>

<svelte:window onmessage={onPluginMessage} />

<div class="space-y-8">
	<div class="flex flex-wrap items-start justify-between gap-4">
		<AdminPageHeading
			title={plugin?.name || pluginId || 'Plugin'}
			subtitle={plugin
				? `${plugin.state}${plugin.version ? ` · v${plugin.version}` : ''} · ${plugin.id}`
				: 'Hosted plugin admin UI'}
			Icon={Blocks}
		/>
		{#if plugin}
			<div class="flex flex-wrap items-center gap-2 pt-1">
				<Badge variant={stateBadgeVariant(plugin.state)}>{plugin.state}</Badge>
				{#if plugin.enabled}
					<Badge>enabled</Badge>
					<Button
						type="button"
						variant="outline"
						disabled={actionBusy}
						onclick={() => void toggleEnabled()}
					>
						{actionBusy ? 'Disabling…' : 'Disable'}
					</Button>
				{:else}
					<Badge variant="outline">disabled</Badge>
					<Button type="button" disabled={actionBusy} onclick={() => void toggleEnabled()}>
						{actionBusy ? 'Enabling…' : 'Enable'}
					</Button>
				{/if}
			</div>
		{/if}
	</div>

	{#if loading}
		<p class="text-muted-foreground text-sm">Loading…</p>
	{:else if err}
		<p class="text-destructive text-sm">{err}</p>
	{/if}

	{#if pluginId}
		<Card.Root>
			<Card.Content class="p-0">
				<iframe
					{@attach attachIframe}
					src={iframeSrc}
					title="{plugin?.name || pluginId} plugin UI"
					sandbox="allow-scripts allow-forms"
					class="bg-background h-[70vh] min-h-[480px] w-full border-0"
					onload={onIframeLoad}
				></iframe>
			</Card.Content>
		</Card.Root>
	{/if}
</div>
