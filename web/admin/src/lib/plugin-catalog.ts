import type { AppConfig } from '$lib/app-config';

export type PluginConfigFieldType = 'string' | 'int' | 'bool' | 'int_list' | 'string_list';

export type PluginConfigField = {
	key: string;
	type: PluginConfigFieldType;
	label: string;
	description?: string;
	default?: unknown;
	validation?: {
		min?: number;
		max?: number;
		min_len?: number;
		max_len?: number;
	};
};

export type PluginConfigSchema = {
	fields: PluginConfigField[];
};

export type PluginCapability =
	| 'read_events'
	| 'write_events'
	| 'delete_events'
	| 'sign_as_relay'
	| 'broadcast'
	| 'validate_event'
	| 'react_event'
	| 'transform_req'
	| 'gate_req_events'
	| 'provide_req_query';

export type PluginRow = {
	id: string;
	nip_number: number;
	title: string;
	description: string;
	url?: string;
	core: boolean;
	mandatory: boolean;
	enabled: boolean;
	default_enabled?: boolean;
	capabilities: PluginCapability[];
	config_schema: PluginConfigSchema;
	settings?: Record<string, unknown> | null;
};

export const PIPELINE_CAPABILITY_WARNINGS: Partial<Record<PluginCapability, string>> = {
	transform_req:
		'This plugin may narrow REQ filters at runtime (intersect-only). Misconfiguration can hide events from subscribers.',
	gate_req_events:
		'This plugin gates which stored events are delivered per subscription. Private reads often require NIP-42 authentication.'
};

const NIP_NUMBER_TO_PLUGIN_ID: Record<number, string> = {
	2: 'nip-02',
	29: 'nip-29',
	50: 'nip-50'
};

/** Plugins shown on Config → Functionalities (NIP-42 lives on Security). */
export function functionalitiesPlugins(plugins: PluginRow[]): PluginRow[] {
	return plugins.filter((p) => p.id !== 'nip-42');
}

export function isPluginEnabledInDraft(cfg: AppConfig, plugin: PluginRow): boolean {
	if (plugin.mandatory) return true;
	if (plugin.id === 'nip-42') return cfg.nip42.enabled;
	if (!plugin.core) return cfg.nips[plugin.id]?.enabled ?? false;
	return plugin.enabled;
}

export function setPluginEnabledInDraft(cfg: AppConfig, plugin: PluginRow, on: boolean): void {
	if (plugin.mandatory) return;
	if (plugin.id === 'nip-42') {
		cfg.nip42.enabled = on;
		return;
	}
	cfg.nips[plugin.id] ??= { enabled: false };
	cfg.nips[plugin.id].enabled = on;
}

export function getPluginSettingValue(
	cfg: AppConfig,
	plugin: PluginRow,
	field: PluginConfigField
): unknown {
	const draftSettings = cfg.nips[plugin.id]?.settings;
	if (draftSettings && field.key in draftSettings) {
		return draftSettings[field.key];
	}
	if (plugin.settings && field.key in plugin.settings) {
		return plugin.settings[field.key];
	}
	return field.default;
}

export function setPluginSettingValue(
	cfg: AppConfig,
	pluginId: string,
	key: string,
	value: unknown
): void {
	cfg.nips[pluginId] ??= { enabled: false };
	cfg.nips[pluginId].settings ??= {};
	cfg.nips[pluginId].settings![key] = value;
}

export function enabledNipNumbersFromDraft(cfg: AppConfig, plugins: PluginRow[]): number[] {
	const nums = new Set<number>();
	for (const p of plugins) {
		if (isPluginEnabledInDraft(cfg, p)) {
			nums.add(p.nip_number);
		}
	}
	return [...nums].sort((a, b) => a - b);
}

export function pluginHasConfigurableFields(plugin: PluginRow): boolean {
	return (plugin.config_schema?.fields?.length ?? 0) > 0 && !plugin.core;
}

export function pipelineCapabilityWarnings(plugin: PluginRow): { cap: PluginCapability; message: string }[] {
	const out: { cap: PluginCapability; message: string }[] = [];
	for (const cap of plugin.capabilities ?? []) {
		const message = PIPELINE_CAPABILITY_WARNINGS[cap];
		if (message) out.push({ cap, message });
	}
	return out;
}

export function nipNumberToPluginId(n: number): string | undefined {
	return NIP_NUMBER_TO_PLUGIN_ID[n];
}
