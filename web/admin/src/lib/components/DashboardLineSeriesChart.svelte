<script lang="ts">
	import * as Chart from '$lib/components/ui/chart/index.js';
	import { LineChart } from 'layerchart';
	import type { ChartConfig } from '$lib/components/ui/chart/index.js';
	import type { ChartPoint } from '$lib/dashboard-metrics.js';

	let {
		data,
		seriesLabel,
		color = 'var(--chart-1)',
		seriesKey = 'main'
	}: {
		data: ChartPoint[];
		seriesLabel: string;
		color?: string;
		seriesKey?: string;
	} = $props();

	const chartConfig: ChartConfig = $derived({
		[seriesKey]: { label: seriesLabel, color }
	});
</script>

{#if data.length === 0}
	<p class="text-muted-foreground py-8 text-center text-sm">No data in this range.</p>
{:else}
	<Chart.Container config={chartConfig} class="aspect-auto h-[220px] w-full max-w-full">
		<LineChart
			data={data}
			x="date"
			y="value"
			axis={true}
			series={[{ key: seriesKey, value: 'value', label: seriesLabel, color }]}
		>
			{#snippet tooltip()}
				<Chart.Tooltip />
			{/snippet}
		</LineChart>
	</Chart.Container>
{/if}
