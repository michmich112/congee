<script lang="ts">
	import './layout.css';
	import favicon from '$lib/assets/favicon.svg';

	let { children } = $props();
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<meta name="color-scheme" content="dark light" />
	<title>Congee admin</title>
</svelte:head>

<div class="bg-background text-foreground min-h-dvh antialiased">
	{@render children()}
</div>
