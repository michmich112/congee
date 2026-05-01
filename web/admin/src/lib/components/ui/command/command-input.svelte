<script lang="ts">
	import { Command as CommandPrimitive } from "bits-ui";
	import { cn } from "$lib/utils.js";
	import Search from "@lucide/svelte/icons/search";

	let {
		ref = $bindable(null),
		class: className,
		value = $bindable(""),
		...restProps
	}: CommandPrimitive.InputProps = $props();
</script>

<div data-slot="command-input-wrapper" class="flex items-center gap-2 border-b px-3 pb-2 pt-1.5">
	<Search class="text-muted-foreground size-4 shrink-0" aria-hidden="true" />
	<CommandPrimitive.Input
		bind:value
		bind:ref
		data-slot="command-input"
		class={cn(
			"placeholder:text-muted-foreground flex h-9 w-full bg-transparent text-sm outline-hidden disabled:cursor-not-allowed disabled:opacity-50",
			className
		)}
		{...restProps}
	/>
</div>
