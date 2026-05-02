<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import type { Snippet } from 'svelte';

	let {
		title,
		description,
		children
	}: {
		title: string;
		description?: string;
		children: Snippet;
	} = $props();
</script>

<Card.Root>
	<Card.Header class="pb-2">
		<Card.Title class="text-base">{title}</Card.Title>
		{#if description}
			<Card.Description>{description}</Card.Description>
		{/if}
	</Card.Header>
	<Card.Content class="pt-0">
		{@render children()}
	</Card.Content>
</Card.Root>
