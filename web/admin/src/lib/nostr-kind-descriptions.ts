/**
 * Kind labels for the admin UI. The catalog is nostr-kinds.json (NIP names, roles).
 * Unknown kinds fall back to NIP-01 storage class.
 */

import catalog from './nostr-kinds.json';

type KindEntry = {
	kind: number;
	nip: string;
	name: string;
	description: string;
	roles?: string[];
};

const WELL_KNOWN_KINDS: Record<number, string> = Object.fromEntries(
	(catalog.kinds as KindEntry[]).map((k) => [k.kind, k.description])
);

function nip01StorageHint(kind: number): string {
	if (kind >= 20000 && kind < 30000) {
		return 'Ephemeral — relays need not store; not in historical queries (NIP-01)';
	}
	if (kind >= 30000 && kind < 40000) {
		return 'Addressable — replaceable per pubkey+kind+d-tag (NIP-01)';
	}
	if (kind === 0 || kind === 3 || (kind >= 10000 && kind < 20000)) {
		return 'Replaceable — newest per pubkey+kind replaces older (NIP-01)';
	}
	if (
		kind === 1 ||
		kind === 2 ||
		(kind >= 4 && kind < 45) ||
		(kind >= 1000 && kind < 10000)
	) {
		return 'Regular — stored independently unless deleted (NIP-01)';
	}
	return 'Treated as regular for storage on this relay (NIP-01)';
}

/** Tooltip text for a kind number: catalog description when known, else NIP-01 class hint. */
export function describeNostrKind(kind: number): string {
	const specific = WELL_KNOWN_KINDS[kind];
	if (specific) {
		return specific;
	}
	return `Kind ${kind} — ${nip01StorageHint(kind)}`;
}

/** Sorted list for audit log kind filter dropdown (number + short description). */
export function knownKindDropdownEntries(): { kind: number; label: string }[] {
	const kinds = Object.keys(WELL_KNOWN_KINDS)
		.map((k) => Number.parseInt(k, 10))
		.filter((n) => Number.isFinite(n))
		.sort((a, b) => a - b);
	return kinds.map((kind) => ({
		kind,
		label: `${kind} — ${WELL_KNOWN_KINDS[kind]}`
	}));
}
