/**
 * Short labels for common Nostr event kinds (admin audit tooltips).
 * Descriptions follow common NIP usage; unknown kinds fall back to NIP-01 storage class.
 */

const WELL_KNOWN_KINDS: Record<number, string> = {
	0: 'Set metadata — replaceable profile for a pubkey (NIP-01)',
	1: 'Short text note (NIP-01)',
	2: 'Relay recommendation (NIP-01)',
	3: 'Follow list / contacts (NIP-02)',
	4: 'Encrypted direct message (NIP-04, legacy)',
	5: 'Event deletion request (NIP-09)',
	6: 'Repost / generic repost (NIP-18)',
	7: 'Reaction — like/dislike/custom emoji (NIP-25)',
	8: 'Badge award (NIP-58)',
	40: 'Channel create (NIP-28)',
	41: 'Channel metadata (NIP-28)',
	42: 'Channel message (NIP-28)',
	43: 'Hide message (NIP-28)',
	44: 'Mute user (NIP-28)',
	1063: 'File metadata (NIP-94)',
	1311: 'Live chat message (NIP-53)',
	1984: 'Issue / label (NIP-32)',
	9734: 'Zap request (NIP-57)',
	9735: 'Zap receipt (NIP-57)',
	10000: 'Mute list (NIP-51)',
	10001: 'Pin list (NIP-51)',
	10002: 'Relay list metadata (NIP-65)',
	10003: 'Bookmarks (NIP-51)',
	30000: 'Addressable event — first d-tag wins (NIP-01)',
	30001: 'Addressable event — first d-tag wins (NIP-01)',
	30023: 'Long-form article (NIP-23)',
	30078: 'Application-specific data (NIP-78)',
	30402: 'Classified listing (NIP-99)',
	31990: 'Handler information (NIP-90)',
	31989: 'Job request (NIP-90)',
	31988: 'Job result (NIP-90)',
	34550: 'Marketplace stall data (NIP-15)',
	34560: 'Product sold as an event (NIP-15)',
	30403: 'Classified listing (reserved; NIP-99)',
	9000: 'Group admin / relay-signed group moderation (NIP-29)',
	9001: 'Group join (NIP-29)',
	39000: 'Group metadata (addressable, NIP-29)',
	39001: 'Group admins (addressable, NIP-29)'
};

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

/** Tooltip text for a kind number: NIP-specific line when known, else NIP-01 class hint. */
export function describeNostrKind(kind: number): string {
	const specific = WELL_KNOWN_KINDS[kind];
	if (specific) {
		return specific;
	}
	return `Kind ${kind} — ${nip01StorageHint(kind)}`;
}
