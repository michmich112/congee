<script lang="ts">
	import * as Chart from '$lib/components/ui/chart/index.js';
	import { LineChart } from 'layerchart';
	import type { ChartConfig } from '$lib/components/ui/chart/index.js';

	export type ConnSeriesPoint = { t: number; auth: number; req: number; ev: number };

	let {
		pts,
		compact = false
	}: {
		pts: ConnSeriesPoint[];
		compact?: boolean;
	} = $props();

	function toDeltaRows(p: ConnSeriesPoint[]) {
		if (p.length === 0) return [];
		const rows: { date: Date; authDelta: number; reqDelta: number; evDelta: number }[] = [];
		for (let i = 0; i < p.length; i++) {
			const prev = i > 0 ? p[i - 1] : { t: p[i].t, auth: 0, req: 0, ev: 0 };
			const cur = p[i];
			rows.push({
				date: new Date(cur.t * 1000),
				authDelta: Math.max(0, cur.auth - prev.auth),
				reqDelta: Math.max(0, cur.req - prev.req),
				evDelta: Math.max(0, cur.ev - prev.ev)
			});
		}
		return rows;
	}

	const data = $derived(toDeltaRows(pts));

	const chartConfig: ChartConfig = $derived({
		auth: { label: 'AUTH', color: 'var(--chart-3)' },
		req: { label: 'REQ', color: 'var(--chart-1)' },
		ev: { label: 'EVENT', color: 'var(--chart-2)' }
	});
</script>

{#if data.length === 0}
	{#if compact}
		<span class="text-muted-foreground text-xs tabular-nums">—</span>
	{:else}
		<p class="text-muted-foreground py-6 text-center text-sm">No samples yet.</p>
	{/if}
{:else}
	<Chart.Container
		config={chartConfig}
		class={compact
			? 'aspect-auto h-10 min-w-[8rem] max-w-[10rem] shrink-0 px-0 py-0'
			: 'aspect-auto h-[240px] w-full max-w-full px-1 py-2 sm:px-2'}
	>
		<LineChart
			data={data}
			x="date"
			yPadding={compact ? [2, 2] : [20, 20]}
			legend={!compact}
			axis={!compact}
			series={[
				{ key: 'auth', value: 'authDelta', label: 'AUTH Δ', color: 'var(--chart-3)' },
				{ key: 'req', value: 'reqDelta', label: 'REQ Δ', color: 'var(--chart-1)' },
				{ key: 'ev', value: 'evDelta', label: 'EVENT Δ', color: 'var(--chart-2)' }
			]}
			props={compact
				? {}
				: {
						yAxis: {
							label: 'Δ / bucket'
						}
					}}
		>
			{#snippet tooltip()}
				<Chart.Tooltip />
			{/snippet}
		</LineChart>
	</Chart.Container>
{/if}
