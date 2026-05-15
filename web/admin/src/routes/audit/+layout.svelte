<script lang="ts">
	import { page } from '$app/state';
	import { cn } from '$lib/utils';

	let { children } = $props();

	function tabClass(href: string) {
		const path = page.url.pathname;
		const active = path === href || path.startsWith(href + '/');
		return cn(
			'rounded-md px-3 py-1.5 text-sm transition-colors',
			active
				? 'bg-muted font-medium text-foreground'
				: 'text-muted-foreground hover:bg-muted/60 hover:text-foreground'
		);
	}
</script>

<div class="space-y-4">
	<nav class="flex flex-wrap gap-1 border-b border-border pb-3" aria-label="Audit sections">
		<a href="/audit/events" class={tabClass('/audit/events')}>Events</a>
		<a href="/audit/connections" class={tabClass('/audit/connections')}>Connections</a>
	</nav>
	{@render children()}
</div>
