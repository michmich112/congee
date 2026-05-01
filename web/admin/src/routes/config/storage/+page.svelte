<script lang="ts">
	import Database from '@lucide/svelte/icons/database';
	import CircleHelp from '@lucide/svelte/icons/circle-help';
	import AdminPageHeading from '$lib/components/AdminPageHeading.svelte';
	import ClipCopy from '$lib/components/ClipCopy.svelte';
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

	const envLockedTooltip =
		'This process uses CONGEE_INSTANCE_ID from the environment. To change it, update the environment variable and restart the relay; it cannot be edited here.';
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
				<div class="space-y-3 md:col-span-2 rounded-lg border border-border bg-muted/20 px-4 py-4">
					<div class="flex flex-wrap items-center gap-2">
						<Label for="relay-instance-id" class="text-sm font-medium">Relay instance ID</Label>
						{#if ctx.relayInstanceRuntime?.env_locked}
							<span
								class="inline-flex text-muted-foreground"
								role="img"
								aria-label={envLockedTooltip}
								title={envLockedTooltip}
							>
								<CircleHelp class="size-4" />
							</span>
						{/if}
					</div>
					<p class="text-muted-foreground text-xs">
						Used as the <code class="rounded bg-muted px-1">origin</code> in PostgreSQL
						<code class="rounded bg-muted px-1">LISTEN</code>/<code class="rounded bg-muted px-1">NOTIFY</code> so
						multi-instance relays do not echo their own writes. Changing this requires a relay restart to apply to
						the database listener. Stored in config as <code class="rounded bg-muted px-1">relay.instance_id</code>.
					</p>
					<div class="flex gap-2">
						<Input
							id="relay-instance-id"
							class="min-w-0 flex-1 font-mono text-sm"
							readonly={ctx.relayInstanceRuntime?.env_locked === true}
							tabindex={ctx.relayInstanceRuntime?.env_locked === true ? -1 : undefined}
							value={ctx.relayInstanceRuntime?.env_locked === true
								? (ctx.relayInstanceRuntime?.instance_id ?? '')
								: (draft().relay.instance_id ?? '')}
							oninput={(e) => {
								if (ctx.relayInstanceRuntime?.env_locked === true) return;
								draft().relay.instance_id = e.currentTarget.value;
								ctx.markDirty();
							}}
						/>
						<ClipCopy
							value={ctx.relayInstanceRuntime?.env_locked === true
								? (ctx.relayInstanceRuntime?.instance_id ?? '')
								: (draft().relay.instance_id ?? '')}
							ariaLabel="Copy relay instance ID"
							title="Copy relay instance ID"
						/>
					</div>
					{#if ctx.relayInstanceRuntime?.env_locked}
						<p class="text-muted-foreground text-xs">
							Set via <code class="rounded bg-muted px-1">CONGEE_INSTANCE_ID</code>.
						</p>
					{/if}
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
