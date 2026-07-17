<script lang="ts">
	import logo from '$lib/assets/congee-logo.svg';
	import { cn } from '$lib/utils';

	let {
		class: className = 'size-8',
		title = 'Congee'
	}: {
		class?: string;
		title?: string;
	} = $props();
</script>

<img src={logo} alt={title} title={title} class={cn('shrink-0 dark:invert', className)} />
