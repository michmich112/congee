<script lang="ts">
	import * as Chart from '$lib/components/ui/chart/index.js';
	import { LineChart } from 'layerchart';
	import type { ChartConfig } from '$lib/components/ui/chart/index.js';
	import type { LatencyChartRow } from '$lib/dashboard-metrics.js';

	let { data }: { data: LatencyChartRow[] } = $props();

	const chartConfig: ChartConfig = $derived({
		mean: { label: 'Mean', color: 'var(--chart-3)' },
		median: { label: 'Median', color: 'var(--chart-4)' },
		p99: { label: 'P99', color: 'var(--chart-5)' },
	});
</script>

{#if data.length === 0}
	<p class="text-muted-foreground py-8 text-center text-sm">No latency samples in this range.</p>
{:else}
	<Chart.Container config={chartConfig} class="aspect-auto h-[260px] w-full max-w-full">
		<LineChart
			data={data}
			x="date"
			legend={true}
			axis={true}
			series={[
				{ key: 'mean', value: 'meanMs', label: 'Mean', color: 'var(--chart-3)' },
				{ key: 'median', value: 'medianMs', label: 'Median', color: 'var(--chart-4)' },
				{ key: 'p99', value: 'p99Ms', label: 'P99', color: 'var(--chart-5)' },
			]}
			props={{
				yAxis: {
					label: 'Latency (ms)',
				},
			}}
		>
			{#snippet tooltip()}
				<Chart.Tooltip />
			{/snippet}
		</LineChart>
	</Chart.Container>
{/if}
