<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import StatInfoIcon from '$lib/components/StatInfoIcon.svelte';
	import { formatCompactCount } from '$lib/format-compact-count';
	import type { Snippet } from 'svelte';

	let {
		label,
		value,
		info,
		compactNumber = false,
		smallNumbers = false,
		fractionDigits,
		badge,
		footer
	}: {
		label: string;
		value: number;
		/** Hover/focus tooltip next to the label. */
		info?: string;
		compactNumber?: boolean;
		smallNumbers?: boolean;
		fractionDigits?: number;
		badge?: Snippet;
		footer?: Snippet;
	} = $props();

	function formatDisplay(): string {
		if (compactNumber) return formatCompactCount(value);
		const fd = fractionDigits ?? 0;
		if (fd === 0 || Number.isInteger(value)) return String(Math.round(value));
		return value.toFixed(fd);
	}
</script>

<Card.Root>
	<Card.Header class="pb-2">
		<div class="flex flex-wrap items-center gap-x-2 gap-y-1">
			<Card.Description>{label}</Card.Description>
			{#if info}
				<StatInfoIcon {info} />
			{/if}
		</div>
		<Card.Title
			class={smallNumbers ? 'text-2xl tabular-nums' : 'text-3xl tabular-nums'}
			data-testid="analytics-stat-value"
		>
			{formatDisplay()}
		</Card.Title>
	</Card.Header>
	{#if badge}
		<Card.Content class="pt-0">
			{@render badge()}
		</Card.Content>
	{/if}
	{#if footer}
		<Card.Content class="text-muted-foreground text-xs">
			{@render footer()}
		</Card.Content>
	{/if}
</Card.Root>
