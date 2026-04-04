<script lang="ts">
	import './layout.css';
	import favicon from '$lib/assets/favicon.svg';

	let { children } = $props();
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>Congee admin</title>
</svelte:head>

<div class="min-h-dvh bg-background text-foreground antialiased">
	<header class="border-b border-border px-6 py-4">
		<p class="text-sm font-medium tracking-tight text-muted-foreground">Congee</p>
		<h1 class="text-lg font-semibold">Relay admin</h1>
	</header>
	<main class="px-6 py-8">
		{@render children()}
	</main>
</div>
