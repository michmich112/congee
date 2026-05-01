<script lang="ts">
	import type { Component } from 'svelte';

	type IconCmp = Component<{ class?: string }>;

	let {
		title,
		subtitle,
		Icon
	}: {
		title: string;
		subtitle?: string;
		Icon: IconCmp;
	} = $props();
</script>

<header class="mb-6 flex flex-col gap-1">
	<div class="flex items-start gap-3">
		<div
			class="bg-muted/80 flex size-10 shrink-0 items-center justify-center rounded-lg border border-border"
			aria-hidden="true"
		>
			<Icon class="text-foreground size-[1.125rem] opacity-90" />
		</div>
		<div class="min-w-0 pt-0.5">
			<h1 class="text-xl font-semibold tracking-tight">{title}</h1>
			{#if subtitle}
				<p class="text-muted-foreground mt-0.5 text-sm">{subtitle}</p>
			{/if}
		</div>
	</div>
</header>
