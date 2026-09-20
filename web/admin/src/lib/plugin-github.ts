const GITHUB_DOWNLOAD =
	/^https:\/\/github\.com\/([^/]+)\/([^/]+)\/releases\/download\/([^/]+)\/([^/?#]+)/i;

export type GitHubReleaseAsset = {
	name: string;
	browser_download_url: string;
	digest?: string;
};

export type GitHubLatestRelease = {
	tag_name?: string;
	assets?: GitHubReleaseAsset[];
};

export type GitHubUpdateCheck =
	| { kind: 'not_github' }
	| { kind: 'error'; error: string }
	| {
			kind: 'ok';
			tag: string;
			newer: boolean;
			url: string;
			sha256: string;
			assetName: string;
	  };

export type PluginUpdateTarget = {
	id: string;
	name?: string;
	version?: string;
	enabled: boolean;
	source_url?: string;
};

export function parseGitHubReleaseDownload(url: string) {
	const m = url.trim().match(GITHUB_DOWNLOAD);
	if (!m) return null;
	return {
		owner: m[1],
		repo: m[2],
		tag: decodeURIComponent(m[3]),
		asset: decodeURIComponent(m[4])
	};
}

export function stripVersionPrefix(v: string) {
	return v.trim().replace(/^v/i, '');
}

export function isNewerVersion(latest: string, current: string) {
	const a = stripVersionPrefix(latest)
		.split('.')
		.map((n) => parseInt(n, 10) || 0);
	const b = stripVersionPrefix(current)
		.split('.')
		.map((n) => parseInt(n, 10) || 0);
	const len = Math.max(a.length, b.length);
	for (let i = 0; i < len; i++) {
		const x = a[i] || 0;
		const y = b[i] || 0;
		if (x > y) return true;
		if (x < y) return false;
	}
	return false;
}

function digestSha256(digest: string | undefined) {
	if (!digest) return '';
	const lower = digest.trim().toLowerCase();
	if (lower.startsWith('sha256:')) return digest.trim().slice(7);
	return '';
}

export async function checkGitHubPluginUpdate(
	sourceUrl: string,
	currentVersion: string
): Promise<GitHubUpdateCheck> {
	const parsed = parseGitHubReleaseDownload(sourceUrl);
	if (!parsed) return { kind: 'not_github' };
	let res: Response;
	try {
		res = await fetch(
			`https://api.github.com/repos/${parsed.owner}/${parsed.repo}/releases/latest`,
			{ headers: { Accept: 'application/vnd.github+json' } }
		);
	} catch (e) {
		return { kind: 'error', error: e instanceof Error ? e.message : 'GitHub request failed' };
	}
	if (!res.ok) {
		return { kind: 'error', error: `GitHub releases: HTTP ${res.status}` };
	}
	const rel = (await res.json()) as GitHubLatestRelease;
	const tag = rel.tag_name || '';
	const assets = rel.assets ?? [];
	const asset =
		assets.find((a) => a.name === parsed.asset) ||
		assets.find((a) => a.name.endsWith(parsed.asset.replace(/^.*?(linux-|darwin-)/, '$1')));
	return {
		kind: 'ok',
		tag,
		newer: isNewerVersion(tag, currentVersion),
		url: asset?.browser_download_url || '',
		sha256: digestSha256(asset?.digest),
		assetName: asset?.name || ''
	};
}
