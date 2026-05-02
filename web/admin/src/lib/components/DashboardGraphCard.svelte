<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import StatInfoIcon from '$lib/components/StatInfoIcon.svelte';
	import type { Snippet } from 'svelte';

	let {
		title,
		description,
		info,
		children
	}: {
		title: string;
		description?: string;
		/** Shown in a hover/focus tooltip next to the title. */
		info?: string;
		children: Snippet;
	} = $props();
</script>

<Card.Root>
	<Card.Header class="pb-2">
		<div class="flex flex-wrap items-center gap-x-2 gap-y-1">
			<Card.Title class="text-base">{title}</Card.Title>
			{#if info}
				<StatInfoIcon {info} />
			{/if}
		</div>
		{#if description}
			<Card.Description>{description}</Card.Description>
		{/if}
	</Card.Header>
	<Card.Content class="pt-0">
		{@render children()}
	</Card.Content>
</Card.Root>
