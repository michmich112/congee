import { adminFetch } from '$lib/admin-api';

export type PluginNavItem = {
	id: string;
	name: string;
	enabled: boolean;
	state: string;
};

export const pluginNav = $state({
	items: [] as PluginNavItem[]
});

export async function refreshPluginNav(): Promise<void> {
	try {
		const res = await adminFetch('/api/plugins');
		if (!res.ok) {
			pluginNav.items = [];
			return;
		}
		const data = (await res.json()) as { plugins?: PluginNavItem[] };
		pluginNav.items = data.plugins ?? [];
	} catch {
		pluginNav.items = [];
	}
}
