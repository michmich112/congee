<script lang="ts">
	import Info from '@lucide/svelte/icons/info';
	import { browser } from '$app/environment';
	import { tick, type Snippet } from 'svelte';
	import { cn } from '$lib/utils.js';

	let { info, content }: { info: string; content?: Snippet } = $props();

	let buttonEl = $state<HTMLButtonElement | null>(null);
	let tooltipEl = $state<HTMLSpanElement | null>(null);
	let open = $state(false);
	let placed = $state(false);
	let top = $state(0);
	/** Viewport X of the tooltip’s horizontal center (tooltip uses -translate-x-1/2). */
	let centerX = $state(0);

	const pad = 8;
	const gap = 8;

	function clampCenterX(x: number, tooltipWidth: number): number {
		const half = tooltipWidth / 2;
		return Math.max(pad + half, Math.min(x, window.innerWidth - pad - half));
	}

	function clampTop(y: number, tooltipHeight: number): number {
		return Math.max(pad, Math.min(y, window.innerHeight - pad - tooltipHeight));
	}

	async function updatePlacement(): Promise<void> {
		if (!browser || !buttonEl || !tooltipEl) return;

		let tw = tooltipEl.offsetWidth;
		let th = tooltipEl.offsetHeight;
		if (tw === 0 || th === 0) {
			await new Promise<void>((r) => requestAnimationFrame(() => r()));
			tw = tooltipEl.offsetWidth;
			th = tooltipEl.offsetHeight;
		}
		if (tw === 0 || th === 0) return;

		const br = buttonEl.getBoundingClientRect();
		const idealCenter = br.left + br.width / 2;

		let y = br.bottom + gap;
		if (y + th > window.innerHeight - pad) {
			y = br.top - gap - th;
		}

		centerX = clampCenterX(idealCenter, tw);
		top = clampTop(y, th);
		placed = true;
	}

	async function show(): Promise<void> {
		open = true;
		placed = false;
		await tick();
		await updatePlacement();
	}

	function hide(): void {
		open = false;
		placed = false;
	}

	function onViewportChange(): void {
		if (open) void updatePlacement();
	}

	$effect(() => {
		if (!browser || !open) return;
		window.addEventListener('resize', onViewportChange);
		window.addEventListener('scroll', onViewportChange, true);
		return () => {
			window.removeEventListener('resize', onViewportChange);
			window.removeEventListener('scroll', onViewportChange, true);
		};
	});
</script>

<!-- No transform on this wrapper: it would make `position:fixed` tooltips use this span as their containing block. -->
<span class="relative top-px inline-flex shrink-0">
	<button
		type="button"
		bind:this={buttonEl}
		class={cn(
			'text-muted-foreground ring-offset-background hover:text-foreground',
			'inline-flex size-6 items-center justify-center rounded-full',
			'outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2'
		)}
		aria-label={info}
		onmouseenter={() => void show()}
		onmouseleave={hide}
		onfocus={() => void show()}
		onblur={hide}
	>
		<Info class="size-3.5" aria-hidden="true" />
	</button>
	{#if open}
		<span
			bind:this={tooltipEl}
			role="tooltip"
			style:left="{centerX}px"
			style:top="{top}px"
			class={cn(
				'border-border bg-popover text-popover-foreground pointer-events-none fixed z-50',
				'w-max max-w-[min(24rem,calc(100vw-2rem))] -translate-x-1/2 rounded-md border px-3 py-2 text-left text-xs leading-snug shadow-md',
				placed ? 'opacity-100' : 'opacity-0'
			)}
		>
			{#if content}
				{@render content()}
			{:else}
				{info}
			{/if}
		</span>
	{/if}
</span>
