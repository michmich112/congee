<script lang="ts">
	import { Label } from '$lib/components/ui/label';
	import {
		setTimestampDisplayMode,
		timestampDisplay
	} from '$lib/admin-timestamp-preference.svelte';
	import type { TimestampDisplayMode } from '$lib/format-timestamp';

	const selectClass =
		'border-input dark:bg-input/30 focus-visible:border-ring focus-visible:ring-ring/50 h-8 rounded-lg border bg-transparent px-2.5 text-sm shadow-xs outline-none focus-visible:ring-3';

	let { selectId }: { selectId: string } = $props();
</script>

<div class="flex flex-wrap items-center gap-2 text-sm">
	<Label for={selectId} class="text-muted-foreground whitespace-nowrap">Timestamps</Label>
	<select
		id={selectId}
		class={selectClass}
		value={timestampDisplay.mode}
		onchange={(e) => {
			const v = e.currentTarget.value;
			if (v === 'unix' || v === 'utc' || v === 'local') {
				setTimestampDisplayMode(v as TimestampDisplayMode);
			}
		}}
	>
		<option value="unix">Unix (ms)</option>
		<option value="utc">UTC</option>
		<option value="local">Local</option>
	</select>
</div>
