<script lang="ts">
	import { formatTimestamp, toEpochMs } from '$lib/format-timestamp';
	import { timestampDisplay } from '$lib/admin-timestamp-preference.svelte';

	let { unixValue }: { unixValue: number } = $props();

	const text = $derived(formatTimestamp(timestampDisplay.mode, unixValue));
	const title = $derived(
		`${toEpochMs(unixValue)} ms since epoch · ${new Date(toEpochMs(unixValue)).toISOString()}`
	);
</script>

<span class="font-mono text-xs tabular-nums whitespace-nowrap" {title}>{text}</span>
