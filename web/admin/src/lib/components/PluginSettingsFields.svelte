<script lang="ts">
	import type { AppConfig } from '$lib/app-config';
	import {
		getPluginSettingValue,
		setPluginSettingValue,
		type PluginConfigField,
		type PluginRow
	} from '$lib/plugin-catalog';
	import { parseIntSafe } from '$lib/app-config';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import { Textarea } from '$lib/components/ui/textarea';

	type Props = {
		cfg: AppConfig;
		plugin: PluginRow;
		disabled?: boolean;
		onchange: () => void;
	};

	let { cfg, plugin, disabled = false, onchange }: Props = $props();

	const fields = $derived(plugin.config_schema?.fields ?? []);

	function fieldId(key: string): string {
		return `${plugin.id}-${key}`;
	}

	function updateSetting(field: PluginConfigField, value: unknown) {
		setPluginSettingValue(cfg, plugin.id, field.key, value);
		onchange();
	}

	function intListToText(value: unknown): string {
		if (!Array.isArray(value)) return '';
		return value.map((v) => String(v)).join(', ');
	}

	function stringListToText(value: unknown): string {
		if (!Array.isArray(value)) return '';
		return value.map((v) => String(v)).join('\n');
	}

	function parseIntList(raw: string): number[] {
		return raw
			.split(/[\s,]+/)
			.map((s) => parseInt(s.trim(), 10))
			.filter((n) => Number.isFinite(n));
	}

	function parseStringList(raw: string): string[] {
		return raw
			.split('\n')
			.map((s) => s.trim())
			.filter(Boolean);
	}
</script>

{#if fields.length > 0}
	<div class="grid gap-4 md:grid-cols-2">
		{#each fields as field (field.key)}
			{@const value = getPluginSettingValue(cfg, plugin, field)}
			{#if field.type === 'bool'}
				<div
					class="flex flex-col gap-3 rounded-lg border border-border bg-muted/30 px-4 py-3 md:col-span-2 sm:flex-row sm:items-center sm:justify-between"
				>
					<div class="space-y-1">
						<Label for={fieldId(field.key)} class="text-sm font-medium {disabled ? 'opacity-50' : ''}">
							{field.label}
						</Label>
						{#if field.description}
							<p class="text-xs text-muted-foreground">{field.description}</p>
						{/if}
					</div>
					<Switch
						id={fieldId(field.key)}
						{disabled}
						checked={Boolean(value)}
						onCheckedChange={(on) => updateSetting(field, on)}
					/>
				</div>
			{:else if field.type === 'int'}
				<div class="space-y-2">
					<Label for={fieldId(field.key)} class={disabled ? 'pointer-events-none opacity-50' : ''}>
						{field.label}
					</Label>
					<Input
						id={fieldId(field.key)}
						type="number"
						min={field.validation?.min}
						max={field.validation?.max}
						{disabled}
						value={String(value ?? '')}
						oninput={(e) => {
							const fallback =
								typeof value === 'number'
									? value
									: typeof field.default === 'number'
										? field.default
										: 0;
							updateSetting(field, parseIntSafe(e.currentTarget.value, fallback));
						}}
					/>
					{#if field.description}
						<p class="text-xs text-muted-foreground">{field.description}</p>
					{/if}
				</div>
			{:else if field.type === 'int_list'}
				<div class="space-y-2 md:col-span-2">
					<Label for={fieldId(field.key)} class={disabled ? 'pointer-events-none opacity-50' : ''}>
						{field.label}
					</Label>
					<Input
						id={fieldId(field.key)}
						class="font-mono text-xs"
						spellcheck={false}
						placeholder="e.g. 4, 40"
						{disabled}
						value={intListToText(value)}
						oninput={(e) => updateSetting(field, parseIntList(e.currentTarget.value))}
					/>
					{#if field.description}
						<p class="text-xs text-muted-foreground">{field.description}</p>
					{/if}
				</div>
			{:else if field.type === 'string_list'}
				<div class="space-y-2 md:col-span-2">
					<Label for={fieldId(field.key)} class={disabled ? 'pointer-events-none opacity-50' : ''}>
						{field.label}
					</Label>
					<Textarea
						id={fieldId(field.key)}
						class="min-h-[100px] font-mono text-xs"
						spellcheck={false}
						{disabled}
						value={stringListToText(value)}
						oninput={(e) => updateSetting(field, parseStringList(e.currentTarget.value))}
					/>
					{#if field.description}
						<p class="text-xs text-muted-foreground">{field.description}</p>
					{/if}
				</div>
			{:else}
				<div class="space-y-2 md:col-span-2">
					<Label for={fieldId(field.key)} class={disabled ? 'pointer-events-none opacity-50' : ''}>
						{field.label}
					</Label>
					<Input
						id={fieldId(field.key)}
						class="font-mono text-xs"
						spellcheck={false}
						{disabled}
						value={String(value ?? '')}
						oninput={(e) => updateSetting(field, e.currentTarget.value)}
					/>
					{#if field.description}
						<p class="text-xs text-muted-foreground">{field.description}</p>
					{/if}
				</div>
			{/if}
		{/each}
	</div>
{/if}
