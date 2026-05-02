<script lang="ts">
	import Info from '@lucide/svelte/icons/info';
	import { cn } from '$lib/utils.js';

	let { info }: { info: string } = $props();
</script>

<span class="group relative inline-flex shrink-0 translate-y-px">
	<button
		type="button"
		class={cn(
			'text-muted-foreground ring-offset-background hover:text-foreground',
			'inline-flex size-6 items-center justify-center rounded-full',
			'outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2'
		)}
		aria-label={info}
	>
		<Info class="size-3.5" aria-hidden="true" />
	</button>
	<span
		role="tooltip"
		class={cn(
			'border-border bg-popover text-popover-foreground pointer-events-none absolute left-1/2 top-full z-50 mt-2',
			'w-max max-w-[min(20rem,calc(100vw-2rem))] -translate-x-1/2 rounded-md border px-3 py-2 text-left text-xs leading-snug shadow-md',
			'invisible opacity-0 transition-[opacity,visibility] duration-150',
			'group-hover:visible group-hover:opacity-100',
			'group-focus-within:visible group-focus-within:opacity-100'
		)}
	>
		{info}
	</span>
</span>
