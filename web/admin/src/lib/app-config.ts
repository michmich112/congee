/** Mirrors `internal/config/config_types.go` JSON shape (admin config file). */
export type AppConfig = {
	relay: { port: number };
	admin: { port: number };
	database: { type: string; dsn: string };
	logging: { level: string; format: string };
	audit: { retention_days: number };
	rate_limits: {
		events_per_minute_per_connection: number;
		bytes_per_second_per_connection: number;
		reqs_per_minute_per_connection: number;
		messages_per_minute_per_ip: number;
	};
	connection_limits: {
		max_open: number;
		max_subscriptions_per_connection: number;
		max_filters_per_req: number;
		connections_per_minute_per_ip: number;
		read_deadline_seconds: number;
		write_deadline_seconds: number;
	};
	websocket: {
		compression_enabled: boolean;
		max_message_bytes: number;
	};
	max_subscription_id_length: number;
	nip11: {
		name: string;
		description: string;
		pubkey: string;
		contact: string;
		supported_nips: number[];
		software: string;
		version: string;
	};
	nips: { enabled: number[] };
};

export function cloneConfig(c: AppConfig): AppConfig {
	return JSON.parse(JSON.stringify(c)) as AppConfig;
}

export function parseConfigJson(text: string): AppConfig {
	const v = JSON.parse(text) as AppConfig;
	if (typeof v !== 'object' || v === null) throw new Error('config root must be an object');
	return v;
}

export function parseIntSafe(raw: string, fallback: number): number {
	const n = parseInt(raw, 10);
	return Number.isFinite(n) ? n : fallback;
}

/** Comma / whitespace separated NIP numbers for NIP-11 metadata. */
export function parseNipIntList(s: string): number[] {
	const seen = new Set<number>();
	for (const part of s.split(/[,\s]+/)) {
		const t = part.trim();
		if (!t) continue;
		const n = parseInt(t, 10);
		if (Number.isFinite(n)) seen.add(n);
	}
	return Array.from(seen).sort((a, b) => a - b);
}
