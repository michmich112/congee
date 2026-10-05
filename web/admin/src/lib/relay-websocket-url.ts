/**
 * One validator for the NIP-42 canonical relay URL and every relay alias.
 * Mirrors `config.NormalizeNIP42RelayURL` (net/url, not the WHATWG URL parser):
 * ws/wss only, host required, scheme and host lowercased, path defaults to "/",
 * trailing slashes other than a lone "/" removed. Default ports, Unicode hosts,
 * and "." / ".." path segments are left as written. Query strings and fragments are dropped.
 */
export function relayWebSocketURL(raw: string): { url: string } | { error: string } {
	const trimmed = raw.trim();
	if (trimmed === '') {
		return { error: 'Enter a WebSocket URL starting with ws:// or wss://.' };
	}
	const schemeMatch = /^([a-zA-Z][a-zA-Z0-9+.-]*):/.exec(trimmed);
	if (!schemeMatch) {
		return { error: 'Enter a valid WebSocket URL.' };
	}
	const scheme = schemeMatch[1].toLowerCase();
	if (scheme !== 'ws' && scheme !== 'wss') {
		return { error: 'Relay URL must use ws:// or wss://.' };
	}
	const rest = trimmed.slice(schemeMatch[0].length);
	if (!rest.startsWith('//')) {
		return { error: 'Relay URL is missing a host.' };
	}
	const afterSlashes = rest.slice(2);
	const hostEnd = afterSlashes.search(/[/?#]/);
	let hostPort = hostEnd === -1 ? afterSlashes : afterSlashes.slice(0, hostEnd);
	const tail = hostEnd === -1 ? '' : afterSlashes.slice(hostEnd);
	const at = hostPort.lastIndexOf('@');
	if (at >= 0) hostPort = hostPort.slice(at + 1);
	if (hostPort === '') {
		return { error: 'Relay URL is missing a host.' };
	}
	let path = tail;
	const cut = path.search(/[?#]/);
	if (cut >= 0) path = path.slice(0, cut);
	if (path === '') path = '/';
	while (path.length > 1 && path.endsWith('/')) {
		path = path.slice(0, -1);
	}
	return { url: `${scheme}://${hostPort.toLowerCase()}${path}` };
}

/** Normalizes canonical URL and aliases with {@link relayWebSocketURL}. Drops repeated aliases. Returns an error message, or null. */
export function applyNip42RelayURLs(
	nip42: { relay_url: string; relay_aliases: string[] },
	requireCanonical: boolean
): string | null {
	const raw = nip42.relay_url.trim();
	if (raw === '') {
		if (requireCanonical) {
			return 'Canonical relay URL is required while NIP-42 is enabled.';
		}
		nip42.relay_url = '';
	} else {
		const parsed = relayWebSocketURL(raw);
		if ('error' in parsed) return `Canonical relay URL: ${parsed.error}`;
		nip42.relay_url = parsed.url;
	}

	const seen = new Set<string>();
	if (nip42.relay_url !== '') seen.add(nip42.relay_url);
	const out: string[] = [];
	for (const alias of nip42.relay_aliases) {
		const parsed = relayWebSocketURL(alias);
		if ('error' in parsed) return `Relay alias: ${parsed.error}`;
		if (seen.has(parsed.url)) continue;
		seen.add(parsed.url);
		out.push(parsed.url);
	}
	nip42.relay_aliases = out;
	return null;
}
