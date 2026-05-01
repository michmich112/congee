<script lang="ts">
	import Database from '@lucide/svelte/icons/database';
	import AdminPageHeading from '$lib/components/AdminPageHeading.svelte';
	import MigrationTool from '$lib/components/MigrationTool.svelte';
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
	<AdminPageHeading
		title="Storage"
		subtitle="Database connection and migration to another backend."
		Icon={Database}
	/>
	<section class="space-y-4">
		<Card.Root>
			<Card.Header>
				<Card.Title class="text-base">Database</Card.Title>
				<Card.Description>SQLite for single-node; PostgreSQL for larger deployments.</Card.Description>
			</Card.Header>
			<Card.Content class="grid gap-4 md:grid-cols-2">
				<div class="space-y-2 md:col-span-1">
					<Label for="db-type">Type</Label>
					<select
						id="db-type"
						class={ctx.selectClass}
						value={draft().database.type}
						onchange={(e) => {
							draft().database.type = e.currentTarget.value;
							ctx.markDirty();
						}}
					>
						<option value="sqlite">sqlite</option>
						<option value="postgres">postgres</option>
					</select>
				</div>
				<div class="space-y-2 md:col-span-2">
					<Label for="db-dsn">DSN</Label>
					<Input
						id="db-dsn"
						class="font-mono text-xs"
						spellcheck={false}
						value={draft().database.dsn}
						oninput={(e) => {
							draft().database.dsn = e.currentTarget.value;
							ctx.markDirty();
						}}
					/>
				</div>
			</Card.Content>
		</Card.Root>
	</section>

	<Separator />

	<section class="space-y-4">
		<h3 class="text-sm font-medium text-muted-foreground">Migration</h3>
		<MigrationTool />
	</section>
</div>
