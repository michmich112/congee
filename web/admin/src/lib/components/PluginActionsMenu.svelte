<script lang="ts">
	import EllipsisVertical from '@lucide/svelte/icons/ellipsis-vertical';
	import { goto } from '$app/navigation';
	import { buttonVariants } from '$lib/components/ui/button';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { cn } from '$lib/utils';

	type PluginRow = {
		id: string;
		name: string;
		enabled: boolean;
	};

	let {
		plugin,
		busy = false,
		includeSettings = true,
		onEnable,
		onDisable,
		onUninstall
	}: {
		plugin: PluginRow;
		busy?: boolean;
		includeSettings?: boolean;
		onEnable: () => void;
		onDisable: () => void;
		onUninstall: () => void;
	} = $props();
</script>

<DropdownMenu.Root>
	<DropdownMenu.Trigger
		class={cn(buttonVariants({ variant: 'outline', size: 'icon' }), 'shrink-0')}
		disabled={busy}
		aria-label="Plugin actions"
	>
		<EllipsisVertical class="size-4" />
	</DropdownMenu.Trigger>
	<DropdownMenu.Content align="end" class="min-w-44">
		<DropdownMenu.Group>
			{#if includeSettings}
				<DropdownMenu.Item onclick={() => void goto(`/plugins/${plugin.id}`)}>
					Settings
				</DropdownMenu.Item>
			{/if}
			{#if plugin.enabled}
				<DropdownMenu.Item disabled={busy} onclick={onDisable}>Disable</DropdownMenu.Item>
			{:else}
				<DropdownMenu.Item disabled={busy} onclick={onEnable}>Enable</DropdownMenu.Item>
			{/if}
		</DropdownMenu.Group>
		<DropdownMenu.Separator />
		<DropdownMenu.Item variant="destructive" disabled={busy} onclick={onUninstall}>
			Uninstall
		</DropdownMenu.Item>
	</DropdownMenu.Content>
</DropdownMenu.Root>
