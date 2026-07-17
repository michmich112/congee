<script lang="ts">
	import './layout.css';
	import CongeeLogo from '$lib/components/CongeeLogo.svelte';
	import Menu from '@lucide/svelte/icons/menu';
	import { browser } from '$app/environment';
	import ChevronLeft from '@lucide/svelte/icons/chevron-left';
	import ChevronRight from '@lucide/svelte/icons/chevron-right';
	import ChevronDown from '@lucide/svelte/icons/chevron-down';
	import LogOut from '@lucide/svelte/icons/log-out';
	import LayoutDashboard from '@lucide/svelte/icons/layout-dashboard';
	import ClipboardList from '@lucide/svelte/icons/clipboard-list';
	import Activity from '@lucide/svelte/icons/activity';
	import Settings from '@lucide/svelte/icons/settings';
	import Network from '@lucide/svelte/icons/network';
	import Database from '@lucide/svelte/icons/database';
	import FileText from '@lucide/svelte/icons/file-text';
	import Radio from '@lucide/svelte/icons/radio';
	import Shield from '@lucide/svelte/icons/shield';
	import Puzzle from '@lucide/svelte/icons/puzzle';
	import type { Component } from 'svelte';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import {
		adminFetch,
		getAdminToken,
		setAdminToken,
		clearAdminToken,
		verifyAdminToken
	} from '$lib/admin-api';
	import { initTimestampDisplayFromStorage } from '$lib/admin-timestamp-preference.svelte';
	import { syncAdminFavicon } from '$lib/admin-favicon';
	import { Toaster } from '$lib/components/ui/sonner';
	import { Button } from '$lib/components/ui/button';
	import * as Collapsible from '$lib/components/ui/collapsible';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as Sheet from '$lib/components/ui/sheet';
	import { buttonVariants } from '$lib/components/ui/button';
	import { cn } from '$lib/utils';
	import { ModeWatcher, mode } from 'mode-watcher';

	let { children } = $props();

	let ready = $state(false);
	let tokenOk = $state(false);
	let passwordInput = $state('');
	let loginErr = $state('');
	let loginBusy = $state(false);
	let relayVersion = $state<string | null>(null);
	let mobileNavOpen = $state(false);
	let configNavOpen = $state(true);
	let auditNavOpen = $state(true);
	let sidebarCollapsed = $state(false);

	const SIDEBAR_COLLAPSED_KEY = 'congee-admin-sidebar-collapsed';

	function readSidebarCollapsedFromStorage(): boolean {
		if (!browser) return false;
		return localStorage.getItem(SIDEBAR_COLLAPSED_KEY) === '1';
	}

	function toggleSidebarCollapsed() {
		sidebarCollapsed = !sidebarCollapsed;
		if (browser) {
			localStorage.setItem(SIDEBAR_COLLAPSED_KEY, sidebarCollapsed ? '1' : '0');
		}
	}

	type IconComponent = Component<{ class?: string }>;

	const mainNav: { href: string; label: string; Icon: IconComponent }[] = [
		{ href: '/', label: 'Dashboard', Icon: LayoutDashboard }
	];

	const auditNav: { href: string; label: string; Icon: IconComponent }[] = [
		{ href: '/audit/events', label: 'Events', Icon: ClipboardList },
		{ href: '/audit/connections', label: 'Connections', Icon: Activity }
	];

	const configNav: { href: string; label: string; Icon: IconComponent }[] = [
		{ href: '/config/network', label: 'Network', Icon: Network },
		{ href: '/config/storage', label: 'Storage', Icon: Database },
		{ href: '/config/logging', label: 'Logging', Icon: FileText },
		{ href: '/config/relay', label: 'Relay', Icon: Radio },
		{ href: '/config/security', label: 'Security', Icon: Shield },
		{ href: '/config/functionalities', label: 'Functionalities', Icon: Puzzle }
	];

	function navLinkActive(href: string) {
		const path = page.url.pathname;
		return href === '/'
			? path === '/'
			: path === href || path.startsWith(href + '/');
	}

	function navLinkClass(href: string, collapsed: boolean) {
		const active = navLinkActive(href);
		return cn(
			'flex items-center gap-2 rounded-md py-1.5 text-sm transition-colors',
			collapsed ? 'justify-center px-0' : 'px-2',
			active
				? 'bg-muted font-medium text-foreground'
				: 'text-muted-foreground hover:bg-muted/60 hover:text-foreground'
		);
	}

	function configChildClass(href: string, collapsed: boolean) {
		const path = page.url.pathname;
		const active = path === href || path.startsWith(href + '/');
		return cn(
			'flex items-center gap-2 rounded-md py-1.5 text-sm transition-colors',
			collapsed ? 'justify-center px-0' : 'pl-3 pr-2',
			active
				? 'bg-muted font-medium text-foreground'
				: 'text-muted-foreground hover:bg-muted/60 hover:text-foreground'
		);
	}

	function configChildActive(href: string) {
		const path = page.url.pathname;
		return path === href || path.startsWith(href + '/');
	}

	$effect(() => {
		if (page.url.pathname.startsWith('/config')) {
			configNavOpen = true;
		}
		if (page.url.pathname.startsWith('/audit')) {
			auditNavOpen = true;
		}
	});

	$effect(() => {
		void mode.current;
		syncAdminFavicon();

		if (!browser) return;

		const media = window.matchMedia('(prefers-color-scheme: dark)');
		const onChange = () => syncAdminFavicon();
		media.addEventListener('change', onChange);
		return () => media.removeEventListener('change', onChange);
	});

	onMount(() => {
		sidebarCollapsed = readSidebarCollapsedFromStorage();
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

	$effect(() => {
		if (!tokenOk) {
			relayVersion = null;
			return;
		}
		let cancelled = false;
		void adminFetch('/api/stats').then(async (r) => {
			if (!r.ok || cancelled) return;
			const j = (await r.json()) as { relay_version?: string };
			if (!cancelled) {
				relayVersion = typeof j.relay_version === 'string' ? j.relay_version : null;
			}
		});
		return () => {
			cancelled = true;
		};
	});
</script>

<svelte:head>
	<meta name="color-scheme" content="dark light" />
	<title>Congee admin</title>
</svelte:head>

<div class="min-h-dvh bg-background text-foreground antialiased">
	{#if !ready}
		<div class="flex min-h-dvh items-center justify-center text-sm text-muted-foreground">Loading…</div>
	{:else if !tokenOk}
		<main class="mx-auto flex max-w-md flex-col gap-6 px-6 py-16">
			<div class="flex items-start gap-3">
				<CongeeLogo class="mt-0.5 size-10 shrink-0" />
				<div>
					<p class="text-sm font-medium tracking-tight text-muted-foreground">Congee</p>
					<h1 class="text-xl font-semibold">Admin sign in</h1>
				</div>
			</div>
			<div>
				<p class="text-sm text-muted-foreground">
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
		<div class="flex min-h-dvh">
			<!-- Desktop sidebar -->
			<aside
				class={cn(
					'border-border bg-muted/15 hidden shrink-0 flex-col border-r transition-[width] duration-200 ease-out md:flex',
					sidebarCollapsed ? 'w-14' : 'w-56'
				)}
				aria-label="Main navigation"
			>
				<div
					class={cn(
						'flex flex-1 flex-col',
						sidebarCollapsed ? 'items-center gap-3 px-1.5 py-3' : 'gap-6 p-4'
					)}
				>
					<div
						class={cn(
							'flex w-full items-center',
							sidebarCollapsed ? 'flex-col gap-2' : 'justify-between gap-2'
						)}
					>
						<div
							class={cn(
								'flex min-w-0 items-center',
								sidebarCollapsed ? 'justify-center' : 'flex-1 gap-2.5'
							)}
						>
							<CongeeLogo class={cn('shrink-0', sidebarCollapsed ? 'size-7' : 'size-8')} />
							{#if !sidebarCollapsed}
								<div class="min-w-0">
									<p class="text-xs font-medium tracking-tight text-muted-foreground">Congee</p>
									<p class="text-sm font-semibold">Relay admin</p>
								</div>
							{/if}
						</div>
						<Button
							variant="ghost"
							size="icon"
							type="button"
							class="shrink-0"
							aria-label={sidebarCollapsed ? 'Expand navigation sidebar' : 'Collapse navigation sidebar'}
							onclick={toggleSidebarCollapsed}
						>
							{#if sidebarCollapsed}
								<ChevronRight class="size-4" />
							{:else}
								<ChevronLeft class="size-4" />
							{/if}
						</Button>
					</div>
					<nav class={cn('flex w-full flex-col', sidebarCollapsed ? 'gap-1' : 'gap-1')}>
						{#each mainNav as item}
							<a
								href={item.href}
								class={navLinkClass(item.href, sidebarCollapsed)}
								aria-current={navLinkActive(item.href) ? 'page' : undefined}
								title={item.label}
							>
								<item.Icon class="size-4 shrink-0 opacity-80" />
								{#if !sidebarCollapsed}
									<span>{item.label}</span>
								{/if}
							</a>
						{/each}
						{#if !sidebarCollapsed}
							<Collapsible.Root bind:open={auditNavOpen} class="space-y-1">
								<Collapsible.Trigger
									class="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm text-muted-foreground hover:bg-muted/60 hover:text-foreground"
								>
									<ClipboardList class="size-4 shrink-0 opacity-80" />
									<span class="flex-1 font-medium">Audit</span>
									<ChevronDown
										class={cn(
											'text-muted-foreground size-4 shrink-0 transition-transform duration-200',
											auditNavOpen ? 'rotate-180' : ''
										)}
									/>
								</Collapsible.Trigger>
								<Collapsible.Content class="flex flex-col gap-0.5 border-border border-l pl-2">
									{#each auditNav as item}
										<a
											href={item.href}
											class={configChildClass(item.href, false)}
											aria-current={configChildActive(item.href) ? 'page' : undefined}
										>
											<item.Icon class="size-3.5 shrink-0 opacity-75" />
											{item.label}
										</a>
									{/each}
								</Collapsible.Content>
							</Collapsible.Root>
							<Collapsible.Root bind:open={configNavOpen} class="space-y-1">
								<Collapsible.Trigger
									class="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm text-muted-foreground hover:bg-muted/60 hover:text-foreground"
								>
									<Settings class="size-4 shrink-0 opacity-80" />
									<span class="flex-1 font-medium">Config</span>
									<ChevronDown
										class={cn(
											'text-muted-foreground size-4 shrink-0 transition-transform duration-200',
											configNavOpen ? 'rotate-180' : ''
										)}
									/>
								</Collapsible.Trigger>
								<Collapsible.Content class="flex flex-col gap-0.5 border-border border-l pl-2">
									{#each configNav as item}
										<a
											href={item.href}
											class={configChildClass(item.href, false)}
											aria-current={configChildActive(item.href) ? 'page' : undefined}
										>
											<item.Icon class="size-3.5 shrink-0 opacity-75" />
											{item.label}
										</a>
									{/each}
								</Collapsible.Content>
							</Collapsible.Root>
						{:else}
							<div class="mx-auto my-0.5 h-px w-6 shrink-0 bg-border" aria-hidden="true"></div>
							{#each auditNav as item}
								<a
									href={item.href}
									class={configChildClass(item.href, true)}
									aria-current={configChildActive(item.href) ? 'page' : undefined}
									title={item.label}
								>
									<item.Icon class="size-4 shrink-0 opacity-75" />
								</a>
							{/each}
							<div class="mx-auto my-0.5 h-px w-6 shrink-0 bg-border" aria-hidden="true"></div>
							{#each configNav as item}
								<a
									href={item.href}
									class={configChildClass(item.href, true)}
									aria-current={configChildActive(item.href) ? 'page' : undefined}
									title={item.label}
								>
									<item.Icon class="size-4 shrink-0 opacity-75" />
								</a>
							{/each}
						{/if}
					</nav>
					<div
						class={cn(
							'mt-auto flex w-full flex-col',
							sidebarCollapsed ? 'items-center gap-2 border-0 pt-1' : 'gap-3 border-border border-t pt-4'
						)}
					>
						<Button
							variant="outline"
							size={sidebarCollapsed ? 'icon' : 'sm'}
							type="button"
							class={cn(sidebarCollapsed ? 'size-8 shrink-0' : 'w-full justify-center')}
							title="Sign out"
							aria-label="Sign out"
							onclick={logout}
						>
							{#if sidebarCollapsed}
								<LogOut class="size-4" />
							{:else}
								Sign out
							{/if}
						</Button>
						{#if relayVersion && !sidebarCollapsed}
							<p class="text-[0.65rem] leading-snug text-muted-foreground">
								Relay <span class="font-mono tabular-nums">{relayVersion}</span>
								<span class="text-muted-foreground/80"> (NIP-11)</span>
							</p>
						{/if}
					</div>
				</div>
			</aside>
			<div class="flex min-w-0 flex-1 flex-col">
				<header class="border-border flex items-center gap-3 border-b px-4 py-3 md:hidden">
					<Sheet.Root bind:open={mobileNavOpen}>
						<Sheet.Trigger
							class={buttonVariants({ variant: 'outline', size: 'icon' })}
							aria-label="Open navigation menu"
						>
							<Menu class="size-4" />
						</Sheet.Trigger>
						<Sheet.Content side="left" class="flex w-[min(100vw-2rem,18rem)] flex-col gap-0 p-0">
							<Sheet.Header class="border-border border-b px-4 py-4 text-left">
								<div class="flex items-center gap-2.5">
									<CongeeLogo class="size-7 shrink-0" />
									<div>
										<Sheet.Title class="text-base">Congee admin</Sheet.Title>
										<Sheet.Description class="text-xs text-muted-foreground">Navigation</Sheet.Description>
									</div>
								</div>
							</Sheet.Header>
							<div class="flex flex-1 flex-col gap-6 overflow-y-auto p-4">
								<nav class="flex flex-col gap-1">
									{#each mainNav as item}
										<a
											href={item.href}
											class={navLinkClass(item.href, false)}
											aria-current={navLinkActive(item.href) ? 'page' : undefined}
											onclick={() => (mobileNavOpen = false)}
										>
											<item.Icon class="size-4 shrink-0 opacity-80" />
											{item.label}
										</a>
									{/each}
									<p
										class="text-muted-foreground flex items-center gap-2 px-2 pt-3 pb-1 text-xs font-medium tracking-wide uppercase"
									>
										<ClipboardList class="size-3.5 shrink-0 opacity-70" />
										Audit
									</p>
									{#each auditNav as item}
										<a
											href={item.href}
											class={configChildClass(item.href, false)}
											aria-current={configChildActive(item.href) ? 'page' : undefined}
											onclick={() => (mobileNavOpen = false)}
										>
											<item.Icon class="size-3.5 shrink-0 opacity-75" />
											{item.label}
										</a>
									{/each}
									<p
										class="text-muted-foreground flex items-center gap-2 px-2 pt-3 pb-1 text-xs font-medium tracking-wide uppercase"
									>
										<Settings class="size-3.5 shrink-0 opacity-70" />
										Config
									</p>
									{#each configNav as item}
										<a
											href={item.href}
											class={configChildClass(item.href, false)}
											aria-current={configChildActive(item.href) ? 'page' : undefined}
											onclick={() => (mobileNavOpen = false)}
										>
											<item.Icon class="size-3.5 shrink-0 opacity-75" />
											{item.label}
										</a>
									{/each}
								</nav>
								<div class="mt-auto flex flex-col gap-3 border-border border-t pt-4">
									<Button variant="outline" size="sm" type="button" onclick={() => { mobileNavOpen = false; logout(); }}>
										Sign out
									</Button>
									{#if relayVersion}
										<p class="text-[0.65rem] text-muted-foreground">
											Relay <span class="font-mono">{relayVersion}</span>
										</p>
									{/if}
								</div>
							</div>
						</Sheet.Content>
					</Sheet.Root>
					<div class="flex min-w-0 flex-1 items-center gap-2.5">
						<CongeeLogo class="size-7 shrink-0" />
						<div class="min-w-0">
							<p class="text-xs text-muted-foreground">Congee</p>
							<p class="truncate text-sm font-semibold">Relay admin</p>
						</div>
					</div>
				</header>

				<main class="mx-auto w-full max-w-5xl flex-1 px-4 py-6 md:px-6 md:py-8">
					{@render children()}
				</main>
			</div>
		</div>
	{/if}
	<ModeWatcher />
	<Toaster richColors closeButton position="top-center" />
</div>
