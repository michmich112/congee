import { getContext } from 'svelte';
import type { AppConfig } from '$lib/app-config';

export const ADMIN_CONFIG_CTX = Symbol('admin-config');

export type ChangelogRow = {
	created_at: number;
	summary: string;
	json_diff: string;
};

export type NipRow = {
	number: number;
	title: string;
	github_url: string;
	mandatory: boolean;
	implemented: boolean;
	enabled: boolean;
};

/** Context provided by `routes/config/+layout.svelte` for subsection pages. */
export type AdminConfigContext = {
	get draft(): AppConfig | null;
	get nipCatalog(): NipRow[];
	get relayIdentity(): { pubkey_hex: string; npub: string } | null;
	get loading(): boolean;
	get saving(): boolean;
	get loadErr(): string | null;
	get changelogErr(): string | null;
	get saveErr(): string | null;
	get saveOk(): boolean;
	get saveMessage(): string | null;
	get changelogLoading(): boolean;
	get changelog(): ChangelogRow[];
	get changelogExpanded(): boolean;
	set changelogExpanded(v: boolean);
	get dirty(): boolean;
	markDirty: () => void;
	setNipEnabled: (list: number[], nip: number, on: boolean, row: NipRow) => number[];
	selectClass: string;
};

export function getAdminConfig(): AdminConfigContext {
	const v = getContext<AdminConfigContext>(ADMIN_CONFIG_CTX);
	if (!v) {
		throw new Error('getAdminConfig() must be used under /config/* layout');
	}
	return v;
}
