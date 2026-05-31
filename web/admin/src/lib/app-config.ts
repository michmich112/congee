/** Matches `internal/config.DefaultQueryLimitIfUnset` (relay when JSON omits the field). */
export const DEFAULT_QUERY_LIMIT_IF_UNSET = 500;

export type NipPluginEntry = {
	enabled: boolean;
	settings?: Record<string, unknown>;
};

/** Mirrors `internal/config/config_types.go` JSON shape (admin config file). */
export type AppConfig = {
	config_version?: number;
	relay: { port: number; instance_id?: string };
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
		default_query_limit?: number | null;
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
		software: string;
		/** When true, relay adds CORS allowing any origin for GET / NIP-11 only. */
		cors_allow_any_origin?: boolean;
	};
	/** NIP-42 client authentication; required fields apply when NIP 42 is enabled. */
	nip42: {
		enabled: boolean;
		relay_url: string;
		send_challenge_on_connect: boolean;
		created_at_skew_seconds: number;
		require_auth_subscribe_kinds: number[];
		require_auth_publish_kinds: number[];
		allowlisted_pubkeys: string[];
	};
	nips: Record<string, NipPluginEntry>;
};

export function cloneConfig(c: AppConfig): AppConfig {
	return JSON.parse(JSON.stringify(c)) as AppConfig;
}

const defaultNip42 = (): AppConfig['nip42'] => ({
	enabled: false,
	relay_url: '',
	send_challenge_on_connect: false,
	created_at_skew_seconds: 600,
	require_auth_subscribe_kinds: [],
	require_auth_publish_kinds: [],
	allowlisted_pubkeys: []
});

/** Ensures nip42 exists for older config files and the config form. */
export function ensureNip42Draft(cfg: AppConfig): void {
	cfg.nip42 ??= defaultNip42();
	if (typeof cfg.nip42.enabled !== 'boolean') {
		cfg.nip42.enabled = false;
	}
	const n = cfg.nip42;
	// Go JSON encodes nil slices as null; the admin form expects arrays.
	if (!Array.isArray(n.require_auth_subscribe_kinds)) {
		n.require_auth_subscribe_kinds = [];
	}
	if (!Array.isArray(n.require_auth_publish_kinds)) {
		n.require_auth_publish_kinds = [];
	}
	if (!Array.isArray(n.allowlisted_pubkeys)) {
		n.allowlisted_pubkeys = [];
	}
}

/** Ensures nips map exists; Go may emit null for an empty map. */
export function ensureNipsDraft(cfg: AppConfig): void {
	if (!cfg.nips || typeof cfg.nips !== 'object' || Array.isArray(cfg.nips)) {
		cfg.nips = {};
	}
	for (const [id, entry] of Object.entries(cfg.nips)) {
		if (!entry || typeof entry !== 'object') {
			cfg.nips[id] = { enabled: false };
			continue;
		}
		if (typeof entry.enabled !== 'boolean') {
			entry.enabled = false;
		}
	}
}

/** Ensures relay.instance_id exists for older config files and the Storage settings form. */
export function ensureRelayDraft(cfg: AppConfig): void {
	cfg.relay ??= { port: 3334 };
	cfg.relay.instance_id ??= '';
}

function migrateLegacyNipsMap(v: Record<string, unknown>): void {
	const nipNumberToPluginId: Record<number, string> = {
		2: 'nip-02',
		29: 'nip-29',
		50: 'nip-50'
	};

	if (v.nip29 && typeof v.nip29 === 'object' && v.nip29 !== null) {
		const legacy = v.nip29 as Record<string, unknown>;
		const nips =
			v.nips && typeof v.nips === 'object' && !Array.isArray(v.nips)
				? (v.nips as Record<string, NipPluginEntry>)
				: {};
		const entry = nips['nip-29'] ?? { enabled: false };
		entry.settings = { ...(entry.settings ?? {}), ...legacy };
		nips['nip-29'] = entry;
		v.nips = nips;
		delete v.nip29;
	}

	const rawNips = v.nips;
	if (rawNips && typeof rawNips === 'object' && !Array.isArray(rawNips)) {
		const probe = rawNips as Record<string, unknown>;
		if (Array.isArray(probe.enabled)) {
			const enabled = probe.enabled as number[];
			const nips: Record<string, NipPluginEntry> = {};
			for (const [key, val] of Object.entries(probe)) {
				if (key === 'enabled') continue;
				if (val && typeof val === 'object') {
					nips[key] = val as NipPluginEntry;
				}
			}
			for (const n of enabled) {
				if (n === 42) {
					const nip42 =
						v.nip42 && typeof v.nip42 === 'object' && v.nip42 !== null
							? (v.nip42 as Record<string, unknown>)
							: {};
					nip42.enabled = true;
					v.nip42 = nip42;
					continue;
				}
				const id = nipNumberToPluginId[n];
				if (id) {
					nips[id] ??= { enabled: false };
					nips[id].enabled = true;
				}
			}
			v.nips = nips;
		}
	}

	if (typeof v.config_version !== 'number') {
		v.config_version = 1;
	}
}

export function parseConfigJson(text: string): AppConfig {
	const v = JSON.parse(text) as Record<string, unknown>;
	if (typeof v !== 'object' || v === null) throw new Error('config root must be an object');
	if (v.nip11 && typeof v.nip11 === 'object' && v.nip11 !== null) {
		const n11 = v.nip11 as Record<string, unknown>;
		delete n11.supported_nips;
		delete n11.version;
	}
	migrateLegacyNipsMap(v);
	const cfg = v as AppConfig;
	ensureNip42Draft(cfg);
	ensureNipsDraft(cfg);
	ensureRelayDraft(cfg);
	return cfg;
}

export function parseIntSafe(raw: string, fallback: number): number {
	const n = parseInt(raw, 10);
	return Number.isFinite(n) ? n : fallback;
}
