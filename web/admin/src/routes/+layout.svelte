<script lang="ts">
	import './layout.css';
	import favicon from '$lib/assets/favicon.svg';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import {
		getAdminToken,
		setAdminToken,
		clearAdminToken,
		verifyAdminToken
	} from '$lib/admin-api';
	import {
		initTimestampDisplayFromStorage,
		setTimestampDisplayMode,
		timestampDisplay
	} from '$lib/admin-timestamp-preference.svelte';
	import type { TimestampDisplayMode } from '$lib/format-timestamp';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';

	let { children } = $props();

	let ready = $state(false);
	let tokenOk = $state(false);
	let passwordInput = $state('');
	let loginErr = $state('');
	let loginBusy = $state(false);

	const nav = [
		{ href: '/', label: 'Dashboard' },
		{ href: '/audit', label: 'Audit' },
		{ href: '/config', label: 'Config' },
		{ href: '/migration', label: 'Migration' }
	];

	function navClass(href: string) {
		const path = page.url.pathname;
		const active = href === '/' ? path === '/' : path === href || path.startsWith(href + '/');
		return active
			? 'font-medium text-foreground'
			: 'text-muted-foreground hover:text-foreground';
	}

	onMount(() => {
		initTimestampDisplayFromStorage();
		const t = getAdminToken();
		if (t) {
			void verifyAdminToken(t).then((ok) => {
				tokenOk = ok;
				if (!ok) clearAdminToken();
				ready = true;
			});
		} else {
			ready = true;
		}
	});

	async function login(e: Event) {
		e.preventDefault();
		loginErr = '';
		const p = passwordInput.trim();
		if (!p) return;
		loginBusy = true;
		try {
			const ok = await verifyAdminToken(p);
			if (!ok) {
				loginErr = 'Invalid password or could not reach /api/stats.';
				return;
			}
			setAdminToken(p);
			passwordInput = '';
			tokenOk = true;
		} finally {
			loginBusy = false;
		}
	}

	function logout() {
		clearAdminToken();
		tokenOk = false;
	}
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<meta name="color-scheme" content="dark light" />
	<title>Congee admin</title>
</svelte:head>

<div class="min-h-dvh bg-background text-foreground antialiased">
	{#if !ready}
		<div class="flex min-h-dvh items-center justify-center text-sm text-muted-foreground">Loading…</div>
	{:else if !tokenOk}
		<main class="mx-auto flex max-w-md flex-col gap-6 px-6 py-16">
			<div>
				<p class="text-sm font-medium tracking-tight text-muted-foreground">Congee</p>
				<h1 class="text-xl font-semibold">Admin sign in</h1>
				<p class="mt-2 text-sm text-muted-foreground">
					Use the same value as <code class="rounded bg-muted px-1">ADMIN_PASSWORD</code>. Appearance follows
					<code class="rounded bg-muted px-1 text-xs">prefers-color-scheme</code>.
				</p>
			</div>
			<form class="flex flex-col gap-4" onsubmit={login}>
				<div class="space-y-2">
					<Label for="admin-pw">Password</Label>
					<Input
						id="admin-pw"
						type="password"
						autocomplete="current-password"
						bind:value={passwordInput}
					/>
				</div>
				{#if loginErr}
					<p class="text-sm text-destructive">{loginErr}</p>
				{/if}
				<Button type="submit" disabled={loginBusy || !passwordInput.trim()}>
					{loginBusy ? 'Checking…' : 'Sign in'}
				</Button>
			</form>
		</main>
	{:else}
		<header class="border-b border-border px-6 py-4">
			<div class="mx-auto flex max-w-5xl flex-wrap items-end justify-between gap-4">
				<div>
					<p class="text-sm font-medium tracking-tight text-muted-foreground">Congee</p>
					<h1 class="text-lg font-semibold">Relay admin</h1>
				</div>
				<div class="flex flex-wrap items-end gap-3">
					<div class="flex flex-col gap-1.5">
						<Label for="admin-ts-mode" class="text-xs text-muted-foreground">Table timestamps</Label>
						<select
							id="admin-ts-mode"
							class="border-input dark:bg-input/30 focus-visible:border-ring focus-visible:ring-ring/50 h-8 rounded-lg border bg-transparent px-2.5 text-sm shadow-xs outline-none focus-visible:ring-3"
							value={timestampDisplay.mode}
							onchange={(e) => {
								const v = e.currentTarget.value;
								if (v === 'unix' || v === 'utc' || v === 'local') {
									setTimestampDisplayMode(v as TimestampDisplayMode);
								}
							}}
						>
							<option value="unix">Unix (ms)</option>
							<option value="utc">UTC</option>
							<option value="local">Local</option>
						</select>
					</div>
					<Button variant="outline" size="sm" type="button" onclick={logout}>Sign out</Button>
				</div>
			</div>
			<nav class="mx-auto mt-4 flex max-w-5xl flex-wrap gap-4 text-sm">
				{#each nav as item}
					<a href={item.href} class={navClass(item.href)}>{item.label}</a>
				{/each}
			</nav>
		</header>
		<main class="mx-auto max-w-5xl px-6 py-8">
			{@render children()}
		</main>
	{/if}
</div>
