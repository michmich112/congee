/** Matches `internal/config.DefaultQueryLimitIfUnset` (relay when JSON omits the field). */
export const DEFAULT_QUERY_LIMIT_IF_UNSET = 500;

/** Matches `internal/config.DefaultQueryPageSizeIfUnset` (relay when JSON omits the field). */
export const DEFAULT_QUERY_PAGE_SIZE_IF_UNSET = 100;

/** Mirrors `internal/config/config_types.go` JSON shape (admin config file). */
export type AppConfig = {
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
		max_open_per_ip: number;
		max_subscriptions_per_connection: number;
		max_filters_per_req: number;
		connections_per_minute_per_ip: number;
		idle_no_event_no_sub_seconds: number;
		read_deadline_seconds: number;
		write_deadline_seconds: number;
		default_query_limit?: number | null;
		query_page_size?: number | null;
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
		relay_url: string;
		send_challenge_on_connect: boolean;
		created_at_skew_seconds: number;
		require_auth_subscribe_kinds: number[];
		require_auth_publish_kinds: number[];
		allowlisted_pubkeys: string[];
	};
	/** NIP-29 relay groups; used when NIP 29 is enabled. */
	nip29: {
		late_publication_max_past_seconds: number;
		strict_previous_same_h: boolean;
	};
	/** NIP-17 private DMs; reject policy applies when NIP 17 is disabled. */
	nip17: {
		reject_gift_wrap_when_disabled: boolean;
	};
	/** NIP-77 negentropy syncing; used when NIP 77 is enabled. */
	nip77: {
		max_records_per_query: number;
		session_idle_timeout_seconds: number;
		frame_size_limit_bytes: number;
		max_concurrent_sessions: number;
		max_concurrent_loads: number;
		neg_open_per_minute_per_connection: number;
		neg_msg_per_minute_per_connection: number;
		backpressure_req_queue_depth: number;
		upstream_enabled: boolean;
		upstream_pause_when_busy: boolean;
		upstreams: Nip77Upstream[];
	};
	nips: { enabled: number[] };
};

export type Nip77Upstream = {
	name: string;
	url: string;
	filters: unknown[];
	interval_seconds: number;
	enabled: boolean;
};

/** Matches `internal/config.DefaultConfig` connection_limits defaults. */
export const DEFAULT_CONNECTION_LIMITS: AppConfig['connection_limits'] = {
	max_open: 10000,
	max_open_per_ip: 20,
	max_subscriptions_per_connection: 20,
	max_filters_per_req: 10,
	connections_per_minute_per_ip: 60,
	idle_no_event_no_sub_seconds: 90,
	read_deadline_seconds: 120,
	write_deadline_seconds: 30
};

export function cloneConfig(c: AppConfig): AppConfig {
	return JSON.parse(JSON.stringify(c)) as AppConfig;
}

const defaultNip42 = (): AppConfig['nip42'] => ({
	relay_url: '',
	send_challenge_on_connect: false,
	created_at_skew_seconds: 600,
	require_auth_subscribe_kinds: [],
	require_auth_publish_kinds: [],
	allowlisted_pubkeys: []
});

const defaultNip29 = (): AppConfig['nip29'] => ({
	late_publication_max_past_seconds: 86400,
	strict_previous_same_h: false
});

const defaultNip17 = (): AppConfig['nip17'] => ({
	reject_gift_wrap_when_disabled: true
});

const defaultNip77 = (): AppConfig['nip77'] => ({
	max_records_per_query: 100_000,
	session_idle_timeout_seconds: 7,
	frame_size_limit_bytes: 1_048_576,
	max_concurrent_sessions: 8,
	max_concurrent_loads: 2,
	neg_open_per_minute_per_connection: 6,
	neg_msg_per_minute_per_connection: 120,
	backpressure_req_queue_depth: 64,
	upstream_enabled: true,
	upstream_pause_when_busy: true,
	upstreams: []
});

/** Ensures nip42 exists for older config files and the config form. */
export function ensureNip42Draft(cfg: AppConfig): void {
	cfg.nip42 ??= defaultNip42();
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

/** Ensures nips.enabled exists; Go may emit null for a nil enabled slice. */
export function ensureNipsDraft(cfg: AppConfig): void {
	cfg.nips ??= { enabled: [] };
	if (!Array.isArray(cfg.nips.enabled)) {
		cfg.nips.enabled = [];
	}
}

/** Ensures relay.instance_id exists for older config files and the Storage settings form. */
export function ensureRelayDraft(cfg: AppConfig): void {
	cfg.relay ??= { port: 3334 };
	cfg.relay.instance_id ??= '';
}

/** Ensures nip29 exists for older config files and the config form. */
export function ensureNip29Draft(cfg: AppConfig): void {
	cfg.nip29 ??= defaultNip29();
}

/** Ensures nip17 exists for older config files and the config form. */
export function ensureNip17Draft(cfg: AppConfig): void {
	cfg.nip17 ??= defaultNip17();
	if (cfg.nip17.reject_gift_wrap_when_disabled === undefined) {
		cfg.nip17.reject_gift_wrap_when_disabled = true;
	}
}

/** Ensures nip77 exists for older config files and the config form. */
export function ensureNip77Draft(cfg: AppConfig): void {
	cfg.nip77 ??= defaultNip77();
	if (!Array.isArray(cfg.nip77.upstreams)) {
		cfg.nip77.upstreams = [];
	}
}

function finiteNumber(v: unknown): v is number {
	return typeof v === 'number' && Number.isFinite(v);
}

/** Backfills connection_limits for older config files missing newer keys. */
export function ensureConnectionLimitsDraft(cfg: AppConfig): void {
	const d = DEFAULT_CONNECTION_LIMITS;
	cfg.connection_limits ??= { ...d };
	const cl = cfg.connection_limits;
	if (!finiteNumber(cl.max_open)) cl.max_open = d.max_open;
	if (!finiteNumber(cl.max_open_per_ip)) cl.max_open_per_ip = d.max_open_per_ip;
	if (!finiteNumber(cl.max_subscriptions_per_connection)) {
		cl.max_subscriptions_per_connection = d.max_subscriptions_per_connection;
	}
	if (!finiteNumber(cl.max_filters_per_req)) cl.max_filters_per_req = d.max_filters_per_req;
	if (!finiteNumber(cl.connections_per_minute_per_ip)) {
		cl.connections_per_minute_per_ip = d.connections_per_minute_per_ip;
	}
	if (!finiteNumber(cl.idle_no_event_no_sub_seconds)) {
		cl.idle_no_event_no_sub_seconds = d.idle_no_event_no_sub_seconds;
	}
	if (!finiteNumber(cl.read_deadline_seconds)) cl.read_deadline_seconds = d.read_deadline_seconds;
	if (!finiteNumber(cl.write_deadline_seconds)) cl.write_deadline_seconds = d.write_deadline_seconds;
}

export function parseConfigJson(text: string): AppConfig {
	const v = JSON.parse(text) as Record<string, unknown>;
	if (typeof v !== 'object' || v === null) throw new Error('config root must be an object');
	if (v.nip11 && typeof v.nip11 === 'object' && v.nip11 !== null) {
		const n11 = v.nip11 as Record<string, unknown>;
		delete n11.supported_nips;
		delete n11.version;
	}
	const cfg = v as AppConfig;
	ensureNip42Draft(cfg);
	ensureNip29Draft(cfg);
	ensureNip17Draft(cfg);
	ensureNip77Draft(cfg);
	ensureNipsDraft(cfg);
	ensureRelayDraft(cfg);
	ensureConnectionLimitsDraft(cfg);
	return cfg;
}

export function parseIntSafe(raw: string, fallback: number): number {
	const n = parseInt(raw, 10);
	return Number.isFinite(n) ? n : fallback;
}
