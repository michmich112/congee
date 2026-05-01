<script lang="ts">
	import { parseIntSafe } from '$lib/app-config';
	import { getAdminConfig } from '$lib/config/admin-config-context';
	import * as Card from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Separator } from '$lib/components/ui/separator';

	const ctx = getAdminConfig();

	function draft() {
		return ctx.draft!;
	}
</script>

<div class="space-y-8">
	<section class="space-y-4">
		<h3 class="text-sm font-medium text-muted-foreground">Logging</h3>
		<Card.Root>
			<Card.Header>
				<Card.Title class="text-base">Log output</Card.Title>
				<Card.Description>Relay process logging format and verbosity.</Card.Description>
			</Card.Header>
			<Card.Content class="grid gap-4 pt-0 md:grid-cols-2">
				<div class="space-y-2">
					<Label for="log-level">Log level</Label>
					<select
						id="log-level"
						class={ctx.selectClass}
						value={draft().logging.level}
						onchange={(e) => {
							draft().logging.level = e.currentTarget.value;
							ctx.markDirty();
						}}
					>
						<option value="debug">debug</option>
						<option value="info">info</option>
						<option value="warn">warn</option>
						<option value="error">error</option>
					</select>
				</div>
				<div class="space-y-2">
					<Label for="log-format">Format</Label>
					<select
						id="log-format"
						class={ctx.selectClass}
						value={draft().logging.format}
						onchange={(e) => {
							draft().logging.format = e.currentTarget.value;
							ctx.markDirty();
						}}
					>
						<option value="json">json</option>
						<option value="console">console</option>
					</select>
				</div>
			</Card.Content>
		</Card.Root>
	</section>

	<Separator />

	<section class="space-y-4">
		<h3 class="text-sm font-medium text-muted-foreground">Audit retention</h3>
		<Card.Root>
			<Card.Header>
				<Card.Title class="text-base">Stored audit log</Card.Title>
				<Card.Description>
					How long relay audit rows are kept in the database (see also the Audit page for live entries).
				</Card.Description>
			</Card.Header>
			<Card.Content class="pt-0">
				<div class="max-w-xs space-y-2">
					<Label for="audit-days">Retention (days)</Label>
					<Input
						id="audit-days"
						type="number"
						min="1"
						value={String(draft().audit.retention_days)}
						oninput={(e) => {
							draft().audit.retention_days = parseIntSafe(
								e.currentTarget.value,
								draft().audit.retention_days
							);
							ctx.markDirty();
						}}
					/>
				</div>
			</Card.Content>
		</Card.Root>
	</section>
</div>
