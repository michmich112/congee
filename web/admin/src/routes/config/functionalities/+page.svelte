<script lang="ts">
	import Puzzle from '@lucide/svelte/icons/puzzle';
	import ChevronDown from '@lucide/svelte/icons/chevron-down';
	import AdminPageHeading from '$lib/components/AdminPageHeading.svelte';
	import PluginSettingsFields from '$lib/components/PluginSettingsFields.svelte';
	import { getAdminConfig } from '$lib/config/admin-config-context';
	import {
		functionalitiesPlugins,
		isPluginEnabledInDraft,
		pipelineCapabilityWarnings,
		pluginHasConfigurableFields,
		type PluginRow
	} from '$lib/plugin-catalog';
	import { Badge } from '$lib/components/ui/badge';
	import * as Alert from '$lib/components/ui/alert';
	import * as Card from '$lib/components/ui/card';
	import * as Collapsible from '$lib/components/ui/collapsible';
	import { Switch } from '$lib/components/ui/switch';
	import { cn } from '$lib/utils';

	const ctx = getAdminConfig();

	let settingsOpen = $state<Record<string, boolean>>({});

	function draft() {
		return ctx.draft!;
	}

	const plugins = $derived(functionalitiesPlugins(ctx.nipCatalog));

	function isEnabled(plugin: PluginRow): boolean {
		return isPluginEnabledInDraft(draft(), plugin);
	}

	function toggleSettingsOpen(id: string, open: boolean) {
		settingsOpen = { ...settingsOpen, [id]: open };
	}

	function hasSignAsRelay(plugin: PluginRow): boolean {
		return plugin.capabilities?.includes('sign_as_relay') ?? false;
	}
</script>

<div class="space-y-6">
	<AdminPageHeading
		title="Functionalities"
		subtitle="Optional NIPs and schema-driven plugin settings. Changes apply when you click Save."
		Icon={Puzzle}
	/>
	<section id="section-nips" class="space-y-4 scroll-mt-8">
		<Card.Root>
			<Card.Header>
				<Card.Title class="text-base">NIP plugins</Card.Title>
				<Card.Description>
					Turn optional protocols on or off here; mandatory NIPs stay enabled. NIP-42 is configured under
					Security.
				</Card.Description>
			</Card.Header>
			<Card.Content class="p-0">
				<ul class="divide-y divide-border">
					{#each plugins as plugin (plugin.id)}
						{@const enabled = isEnabled(plugin)}
						{@const configurable = pluginHasConfigurableFields(plugin)}
						{@const capWarnings = pipelineCapabilityWarnings(plugin)}
						{@const open = settingsOpen[plugin.id] ?? false}
						{#if configurable || capWarnings.length > 0 || hasSignAsRelay(plugin)}
							<li class="block">
								<Collapsible.Root
									open={open}
									onOpenChange={(v) => toggleSettingsOpen(plugin.id, v)}
									class="block"
								>
									<div class="flex flex-wrap items-center justify-between gap-4 px-4 py-3">
										<div class="min-w-0 flex-1 space-y-1">
											<div class="flex flex-wrap items-center gap-2">
												{#if plugin.url}
													<a
														href={plugin.url}
														class="font-mono text-sm font-medium text-primary underline-offset-4 hover:underline"
														target="_blank"
														rel="noreferrer">NIP-{plugin.nip_number}</a
													>
												{:else}
													<span class="font-mono text-sm font-medium">NIP-{plugin.nip_number}</span>
												{/if}
												{#if plugin.mandatory}
													<Badge variant="secondary">mandatory</Badge>
												{:else if enabled}
													<Badge>enabled</Badge>
												{:else}
													<Badge variant="outline">disabled</Badge>
												{/if}
											</div>
											<p class="text-sm text-muted-foreground">{plugin.title}</p>
											{#if plugin.description}
												<p class="text-xs text-muted-foreground">{plugin.description}</p>
											{/if}
										</div>
										<div class="flex items-center gap-2">
											{#if configurable || hasSignAsRelay(plugin) || capWarnings.length > 0}
												<Collapsible.Trigger
													class="inline-flex h-9 items-center gap-1.5 rounded-md border border-border bg-background px-3 text-sm font-medium hover:bg-muted/60"
												>
													<ChevronDown
														class={cn(
															'text-muted-foreground size-4 shrink-0 transition-transform duration-200',
															open ? 'rotate-180' : ''
														)}
													/>
													Settings
												</Collapsible.Trigger>
											{/if}
											{#if plugin.mandatory}
												<span class="text-sm text-muted-foreground">Always on</span>
											{:else if !plugin.core}
												<Switch
													id="nip-{plugin.id}"
													checked={enabled}
													disabled={plugin.mandatory}
													aria-label={`Enable NIP-${plugin.nip_number} in configuration`}
													onCheckedChange={(on) => {
														ctx.setNipEnabled(plugin, on);
														ctx.markDirty();
													}}
												/>
											{/if}
										</div>
									</div>
									<Collapsible.Content>
										<div class="space-y-4 border-t border-border bg-muted/15 px-4 py-4">
											{#each capWarnings as w (w.cap)}
												<Alert.Root variant="default" class="border-amber-500/50 bg-amber-500/10">
													<Alert.Title class="font-mono text-sm">{w.cap}</Alert.Title>
													<Alert.Description>{w.message}</Alert.Description>
												</Alert.Root>
											{/each}
											{#if configurable}
												{#if !enabled}
													<p class="text-sm text-muted-foreground">
														Enable NIP-{plugin.nip_number} with the toggle above to change these options.
													</p>
												{/if}
												<PluginSettingsFields
													cfg={draft()}
													{plugin}
													disabled={!enabled}
													onchange={() => ctx.markDirty()}
												/>
											{/if}
											{#if hasSignAsRelay(plugin)}
												<div
													class="space-y-2 rounded-lg border border-border bg-muted/30 px-4 py-3 md:col-span-2"
												>
													<p class="text-sm font-medium">Relay signing identity (read-only)</p>
													<p class="text-xs text-muted-foreground">
														Same keypair as NIP-11 and relay-signed events (<code
															class="rounded bg-muted px-1 text-[0.7rem]">relay.secrets.json</code
														>).
													</p>
													{#if ctx.relayIdentity}
														<p class="mt-2 font-mono text-xs break-all text-foreground">
															<span class="text-muted-foreground">npub</span>
															{ctx.relayIdentity.npub}
														</p>
														<p class="mt-1 font-mono text-xs break-all text-muted-foreground">
															<span class="text-muted-foreground">pubkey</span>
															{ctx.relayIdentity.pubkey_hex}
														</p>
													{:else}
														<p class="mt-2 text-xs text-muted-foreground">
															Not available (check admin API / relay identity file).
														</p>
													{/if}
												</div>
											{/if}
										</div>
									</Collapsible.Content>
								</Collapsible.Root>
							</li>
						{:else}
							<li class="flex flex-wrap items-center justify-between gap-4 px-4 py-3">
								<div class="min-w-0 flex-1 space-y-1">
									<div class="flex flex-wrap items-center gap-2">
										{#if plugin.url}
											<a
												href={plugin.url}
												class="font-mono text-sm font-medium text-primary underline-offset-4 hover:underline"
												target="_blank"
												rel="noreferrer">NIP-{plugin.nip_number}</a
											>
										{:else}
											<span class="font-mono text-sm font-medium">NIP-{plugin.nip_number}</span>
										{/if}
										{#if plugin.mandatory}
											<Badge variant="secondary">mandatory</Badge>
										{:else if enabled}
											<Badge>enabled</Badge>
										{:else}
											<Badge variant="outline">disabled</Badge>
										{/if}
									</div>
									<p class="text-sm text-muted-foreground">{plugin.title}</p>
								</div>
								<div class="flex items-center gap-3">
									{#if plugin.mandatory}
										<span class="text-sm text-muted-foreground">Always on</span>
									{:else if !plugin.core}
										<Switch
											id="nip-{plugin.id}"
											checked={enabled}
											disabled={plugin.mandatory}
											aria-label={`Enable NIP-${plugin.nip_number} in configuration`}
											onCheckedChange={(on) => {
												ctx.setNipEnabled(plugin, on);
												ctx.markDirty();
											}}
										/>
									{/if}
								</div>
							</li>
						{/if}
					{/each}
				</ul>
			</Card.Content>
		</Card.Root>
	</section>
</div>
