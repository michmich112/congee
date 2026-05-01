<script lang="ts">
	import Copy from '@lucide/svelte/icons/copy';
	import { Button } from '$lib/components/ui/button';
	import { cn } from '$lib/utils';
	import { toast } from 'svelte-sonner';

	let {
		value,
		ariaLabel = 'Copy to clipboard',
		title: titleAttr,
		successMessage = 'Copied to clipboard',
		errorMessage = 'Could not copy',
		class: className,
		disabled: disabledProp
	}: {
		/** Text written to the clipboard on success. */
		value: string;
		ariaLabel?: string;
		/** Native `title` tooltip; defaults to `ariaLabel`. */
		title?: string;
		successMessage?: string;
		errorMessage?: string;
		class?: string;
		disabled?: boolean;
	} = $props();

	const disabled = $derived(disabledProp === true || value.length === 0);

	async function copy() {
		if (disabled) return;
		try {
			await navigator.clipboard.writeText(value);
			toast.success(successMessage);
		} catch {
			toast.error(errorMessage);
		}
	}
</script>

<Button
	type="button"
	variant="outline"
	size="icon"
	class={cn('shrink-0', className)}
	disabled={disabled}
	onclick={() => void copy()}
	aria-label={ariaLabel}
	title={titleAttr ?? ariaLabel}
>
	<Copy class="size-4" />
</Button>
