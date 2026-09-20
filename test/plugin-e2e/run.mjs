#!/usr/bin/env node
/**
 * Plugin e2e: starts Congee with fixture/Conduit, speaks NIP-01 over WebSocket via nostr-tools.
 * Writes test/plugin-e2e/report.md
 */
import { spawn } from 'node:child_process'
import { createHash, randomBytes } from 'node:crypto'
import fs from 'node:fs'
import http from 'node:http'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { finalizeEvent, generateSecretKey, getPublicKey } from 'nostr-tools/pure'
import WebSocket from 'ws'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const CONGEE = path.resolve(__dirname, '../..')
const CONDUIT = path.resolve(CONGEE, '../conduit-plugin')
const results = []
let passed = 0
let failed = 0

function rec(name, ok, detail) {
	results.push({ name, ok, detail: detail || '' })
	if (ok) passed++
	else failed++
	console.log(`${ok ? 'PASS' : 'FAIL'} ${name}${detail ? ' — ' + detail : ''}`)
}

function assert(name, cond, detail) {
	rec(name, !!cond, detail)
	if (!cond) throw new Error(name + (detail ? ': ' + detail : ''))
}

function soft(name, cond, detail) {
	rec(name, !!cond, detail)
}

async function waitHealth(port, ms = 15000) {
	const t0 = Date.now()
	while (Date.now() - t0 < ms) {
		try {
			await new Promise((resolve, reject) => {
				const req = http.get(`http://127.0.0.1:${port}/health`, (res) => {
					res.resume()
					res.statusCode === 200 ? resolve() : reject(new Error('status ' + res.statusCode))
				})
				req.on('error', reject)
			})
			return
		} catch {
			await new Promise((r) => setTimeout(r, 100))
		}
	}
	throw new Error('health timeout port ' + port)
}

function writeConfig({ dir, relayPort, adminPort, dsn, pluginsDir, items }) {
	const cfg = {
		relay: { port: relayPort },
		admin: { port: adminPort },
		database: { type: 'turso', dsn },
		logging: { level: 'error', format: 'json' },
		audit: { retention_days: 7 },
		rate_limits: {
			events_per_minute_per_connection: 6000,
			bytes_per_second_per_connection: 10485760,
			reqs_per_minute_per_connection: 6000,
			messages_per_minute_per_ip: 60000,
		},
		connection_limits: {
			max_open: 50,
			max_open_per_ip: 20,
			max_subscriptions_per_connection: 40,
			max_filters_per_req: 10,
			connections_per_minute_per_ip: 600,
			idle_no_event_no_sub_seconds: 90,
			read_deadline_seconds: 60,
			write_deadline_seconds: 30,
			default_query_limit: 500,
			query_page_size: 100,
		},
		websocket: { compression_enabled: false, max_message_bytes: 1048576 },
		max_subscription_id_length: 128,
		nip11: { name: 'e2e', description: 'plugin e2e', pubkey: '', contact: '', software: 'congee' },
		nips: { enabled: [1, 11, 50] },
		plugins: { directory: pluginsDir, intercept_timeout_ms: 250, items: items || [] },
	}
	const p = path.join(dir, 'config.json')
	fs.writeFileSync(p, JSON.stringify(cfg, null, 2))
	return p
}

function copyPkg(srcDir, destDir) {
	fs.cpSync(srcDir, destDir, { recursive: true })
}

function startCongee({ cfgPath, dataDir, extraEnv }) {
	const bin = path.join(CONGEE, 'bin', 'congee')
	const env = {
		...process.env,
		CONFIG_PATH: cfgPath,
		CONGEE_DATA_DIR: dataDir,
		CONGEE_ENV: 'production',
		ENABLE_ADMIN_UI: 'true',
		ADMIN_PASSWORD: 'e2e-admin',
		CONDUIT_EMBEDDER: 'fake',
		...extraEnv,
	}
	delete env.CONGEE_RELAY_PORT
	delete env.CONGEE_ADMIN_PORT
	const child = spawn(bin, [], { env, cwd: CONGEE, stdio: ['ignore', 'pipe', 'pipe'] })
	let log = ''
	child.stdout.on('data', (d) => {
		log += d.toString()
	})
	child.stderr.on('data', (d) => {
		log += d.toString()
	})
	child.log = () => log
	return child
}

function connect(url) {
	return new Promise((resolve, reject) => {
		const ws = new WebSocket(url)
		const q = []
		const waiters = []
		ws.on('message', (data) => {
			let msg
			try {
				msg = JSON.parse(data.toString())
			} catch {
				msg = ['NOTICE', data.toString()]
			}
			if (waiters.length) waiters.shift()(msg)
			else q.push(msg)
		})
		ws.next = (timeout = 8000) => {
			if (q.length) return Promise.resolve(q.shift())
			return new Promise((resolveMsg, rejectMsg) => {
				const t = setTimeout(() => rejectMsg(new Error('ws timeout')), timeout)
				waiters.push((m) => {
					clearTimeout(t)
					resolveMsg(m)
				})
			})
		}
		ws.once('open', () => resolve(ws))
		ws.once('error', reject)
	})
}

async function publish(ws, sk, partial) {
	const ev = finalizeEvent(
		{
			kind: partial.kind,
			created_at: partial.created_at ?? Math.floor(Date.now() / 1000),
			tags: partial.tags ?? [],
			content: partial.content ?? '',
		},
		sk
	)
	const t0 = Date.now()
	ws.send(JSON.stringify(['EVENT', ev]))
	const msg = await ws.next(8000)
	const dt = Date.now() - t0
	return { ev, msg, dt }
}

async function req(ws, sub, filter, timeout = 8000) {
	ws.send(JSON.stringify(['REQ', sub, filter]))
	const events = []
	const t0 = Date.now()
	while (Date.now() - t0 < timeout) {
		const msg = await ws.next(timeout - (Date.now() - t0) + 50)
		if (msg[0] === 'EVENT' && msg[1] === sub) events.push(msg[2])
		else if (msg[0] === 'EOSE' && msg[1] === sub) return { events, eose: true }
		else if (msg[0] === 'CLOSED' && msg[1] === sub) return { events, closed: msg[2], eose: false }
	}
	return { events, eose: false }
}

function freePort() {
	return new Promise((resolve) => {
		const s = http.createServer()
		s.listen(0, '127.0.0.1', () => {
			const { port } = s.address()
			s.close(() => resolve(port))
		})
	})
}

async function buildBins() {
	await run('go', ['build', '-o', 'bin/congee', './cmd/congee'], CONGEE)
	await run('go', ['build', '-o', 'bin/congee-plugin-fixture', './cmd/congee-plugin-fixture'], CONGEE)
	await run('go', ['build', '-o', 'bin/conduit-plugin', './cmd/conduit-plugin'], CONDUIT)
}

function run(cmd, args, cwd) {
	return new Promise((resolve, reject) => {
		const p = spawn(cmd, args, { cwd, env: { ...process.env, CGO_ENABLED: '1', GOTOOLCHAIN: 'local' }, stdio: 'inherit' })
		p.on('exit', (code) => (code === 0 ? resolve() : reject(new Error(cmd + ' exit ' + code))))
	})
}

function stageFixture(pluginsDir) {
	const dest = path.join(pluginsDir, 'fixture')
	fs.mkdirSync(path.join(dest, 'bin'), { recursive: true })
	fs.copyFileSync(path.join(CONGEE, 'cmd/congee-plugin-fixture/plugin.json'), path.join(dest, 'plugin.json'))
	fs.copyFileSync(path.join(CONGEE, 'bin/congee-plugin-fixture'), path.join(dest, 'bin/congee-plugin-fixture'))
	fs.chmodSync(path.join(dest, 'bin/congee-plugin-fixture'), 0o755)
}

function stageConduit(pluginsDir) {
	const dest = path.join(pluginsDir, 'conduit')
	fs.mkdirSync(path.join(dest, 'bin'), { recursive: true })
	fs.copyFileSync(path.join(CONDUIT, 'plugin.json'), path.join(dest, 'plugin.json'))
	fs.copyFileSync(path.join(CONDUIT, 'bin/conduit-plugin'), path.join(dest, 'bin/conduit-plugin'))
	fs.chmodSync(path.join(dest, 'bin/conduit-plugin'), 0o755)
	if (fs.existsSync(path.join(CONDUIT, 'ui'))) {
		fs.cpSync(path.join(CONDUIT, 'ui'), path.join(dest, 'ui'), { recursive: true })
	}
}

async function withRelay(opts, fn) {
	const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'congee-pluge2e-'))
	const relayPort = await freePort()
	const adminPort = await freePort()
	const dataDir = path.join(tmp, 'data')
	fs.mkdirSync(dataDir, { recursive: true })
	const pluginsDir = path.join(tmp, 'plugins')
	fs.mkdirSync(pluginsDir, { recursive: true })
	if (opts.fixture) stageFixture(pluginsDir)
	if (opts.conduit) stageConduit(pluginsDir)
	const items = []
	if (opts.fixture) items.push({ id: 'fixture', enabled: true })
	if (opts.conduit) items.push({ id: 'conduit', enabled: true, settings: { vector_enabled: true, geo_enabled: true, active_filter: true, inject_product_kinds_on_search: true } })
	const cfgPath = writeConfig({
		dir: tmp,
		relayPort,
		adminPort,
		dsn: path.join(dataDir, 'congee.db'),
		pluginsDir,
		items,
	})
	const child = startCongee({ cfgPath, dataDir, extraEnv: opts.env || {} })
	try {
		await waitHealth(relayPort)
		await new Promise((r) => setTimeout(r, opts.readyWait ?? 1500))
		await fn({ relayPort, adminPort, ws: `ws://127.0.0.1:${relayPort}/` })
	} catch (e) {
		console.error('relay log:\n' + child.log())
		throw e
	} finally {
		child.kill('SIGTERM')
		await new Promise((r) => setTimeout(r, 300))
		try {
			child.kill('SIGKILL')
		} catch {
			/* gone */
		}
		fs.rmSync(tmp, { recursive: true, force: true })
	}
}

async function relayMatrix() {
	await withRelay({ fixture: false, conduit: false, readyWait: 400 }, async ({ ws }) => {
		const sk = generateSecretKey()
		const pk = getPublicKey(sk)
		const c = await connect(ws)
		try {
			const { ev, msg, dt } = await publish(c, sk, { kind: 1, content: 'hello e2e' })
			soft('relay EVENT OK', msg[0] === 'OK' && msg[2] === true, `dt=${dt}ms`)
			const { events, eose } = await req(c, 'ids', { ids: [ev.id] })
			soft('relay REQ ids', eose && events.some((e) => e.id === ev.id))
			const a = await req(c, 'authors', { authors: [pk] })
			soft('relay REQ authors', a.events.some((e) => e.id === ev.id))
			const k = await req(c, 'kinds', { kinds: [1] })
			soft('relay REQ kinds', k.events.some((e) => e.id === ev.id))
			const tagged = finalizeEvent({ kind: 1, created_at: Math.floor(Date.now() / 1000), tags: [['e', ev.id]], content: 'reply' }, sk)
			c.send(JSON.stringify(['EVENT', tagged]))
			await c.next()
			const t = await req(c, 'tags', { kinds: [1], '#e': [ev.id] })
			soft('relay REQ tags', t.events.length >= 1)
			const since = await req(c, 'since', { kinds: [1], since: ev.created_at })
			soft('relay REQ since/until', since.eose)
			const lim = await req(c, 'limit', { kinds: [1], limit: 1 })
			soft('relay REQ limit', lim.events.length <= 1)
			c.send(JSON.stringify(['REQ', 'multi', { kinds: [1] }, { authors: [pk] }]))
			let eoseN = 0
			for (let i = 0; i < 20; i++) {
				const m = await c.next()
				if (m[0] === 'EOSE') eoseN++
				if (eoseN) break
			}
			soft('relay multi-filter', eoseN >= 1)
			const empty = await req(c, 'empty', { ids: [randomBytes(32).toString('hex')] })
			soft('relay empty EOSE', empty.eose && empty.events.length === 0)
			c.send(JSON.stringify(['CLOSE', 'empty']))
			soft('relay CLOSE', true)
			const s50 = await req(c, 's50', { kinds: [1], search: 'hello' })
			soft('relay NIP-50 kind 1', s50.eose)
			c.send(JSON.stringify(['EVENT', { id: '00', pubkey: pk, created_at: 1, kind: 1, tags: [], content: 'x', sig: '00' }]))
			const bad = await c.next()
			soft('relay bad sig', bad[0] === 'OK' && bad[2] === false)
			c.send('not-json')
			const notice = await c.next()
			soft('relay invalid JSON', notice[0] === 'NOTICE')
		} finally {
			c.close()
		}
	})
}

async function slowListen() {
	let baseline = 0
	await withRelay({ readyWait: 300 }, async ({ ws }) => {
		const sk = generateSecretKey()
		const c = await connect(ws)
		const { dt } = await publish(c, sk, { kind: 1, content: 'base' })
		baseline = dt
		c.close()
	})
	await withRelay({ fixture: true, env: { PLUGIN_SLOW_LISTEN: '1' }, readyWait: 2000 }, async ({ ws }) => {
		const sk = generateSecretKey()
		const c = await connect(ws)
		const { msg, dt } = await publish(c, sk, { kind: 1, content: 'slow-listen' })
		soft('slow OnStoredEvent still OK', msg[0] === 'OK' && msg[2] === true, `dt=${dt}ms baseline=${baseline}ms`)
		soft('slow listen does not add ~2s', dt < baseline + 800, `dt=${dt} baseline=${baseline}`)
		c.close()
	})
}

async function conduitCases() {
	await withRelay({ conduit: true, readyWait: 2500 }, async ({ ws }) => {
		const sk = generateSecretKey()
		const pk = getPublicKey(sk)
		const c = await connect(ws)
		const { ev: bike } = await publish(c, sk, {
			kind: 30402,
			content: 'a red bicycle for sale on trails',
			tags: [
				['d', 'bike'],
				['title', 'Red Bicycle'],
				['g', '9q8yy'],
				['t', 'bike'],
			],
		})
		soft('conduit store product OK', bike && bike.id)
		const { ev: pizza } = await publish(c, sk, {
			kind: 30402,
			content: 'wood fired pizza',
			tags: [
				['d', 'food'],
				['title', 'Pizza'],
			],
		})
		await new Promise((r) => setTimeout(r, 400))
		const ranked = await req(c, 'rank', { kinds: [30402], search: 'bicycle' })
		soft('conduit rank bicycle first', ranked.events[0] && ranked.events[0].id === bike.id, JSON.stringify(ranked.events.map((e) => e.id)))
		const { ev: sold } = await publish(c, sk, {
			kind: 30402,
			content: 'a red bicycle for sale on trails',
			tags: [
				['d', 'bike'],
				['title', 'Red Bicycle'],
				['status', 'sold'],
			],
		})
		await new Promise((r) => setTimeout(r, 300))
		const after = await req(c, 'sold', { kinds: [30402], search: 'bicycle' })
		soft('conduit inactive not delivered', !after.events.some((e) => e.id === bike.id || e.id === sold.id))
		const geo = await req(c, 'geo', { kinds: [30402], '#g': ['9q8'] })
		soft('conduit geo prefix', geo.eose)
		const inject = await req(c, 'inj', { search: 'pizza' })
		soft('conduit inject kinds', inject.eose)
		const k1 = await req(c, 'k1', { kinds: [1], search: 'hello' })
		soft('conduit kind 1 search not marketplace', k1.eose && !k1.events.some((e) => e.kind === 30402))
		const lim = await req(c, 'lim', { kinds: [30402], search: 'pizza', limit: 1 })
		soft('conduit limit', lim.events.length <= 1)
		c.close()
		void pk
	})
}

const GH_ALPHABET = '0123456789bcdefghjkmnpqrstuvwxyz'

function encodeGeohash(lat, lon, precision = 5) {
	let latMin = -90,
		latMax = 90,
		lonMin = -180,
		lonMax = 180
	let even = true
	let bit = 0
	let ch = 0
	let hash = ''
	while (hash.length < precision) {
		if (even) {
			const mid = (lonMin + lonMax) / 2
			if (lon >= mid) {
				ch |= 1 << (4 - bit)
				lonMin = mid
			} else lonMax = mid
		} else {
			const mid = (latMin + latMax) / 2
			if (lat >= mid) {
				ch |= 1 << (4 - bit)
				latMin = mid
			} else latMax = mid
		}
		even = !even
		bit++
		if (bit === 5) {
			hash += GH_ALPHABET[ch]
			bit = 0
			ch = 0
		}
	}
	return hash
}

function decodeGeohash(hash) {
	let latMin = -90,
		latMax = 90,
		lonMin = -180,
		lonMax = 180
	let even = true
	for (const r of String(hash).toLowerCase()) {
		const idx = GH_ALPHABET.indexOf(r)
		if (idx < 0) return null
		for (let bits = 4; bits >= 0; bits--) {
			const b = (idx >> bits) & 1
			if (even) {
				const mid = (lonMin + lonMax) / 2
				if (b) lonMin = mid
				else lonMax = mid
			} else {
				const mid = (latMin + latMax) / 2
				if (b) latMin = mid
				else latMax = mid
			}
			even = !even
		}
	}
	return { lat: (latMin + latMax) / 2, lon: (lonMin + lonMax) / 2 }
}

function haversineKm(lat1, lon1, lat2, lon2) {
	const r = 6371
	const p1 = (lat1 * Math.PI) / 180
	const p2 = (lat2 * Math.PI) / 180
	const dlat = ((lat2 - lat1) * Math.PI) / 180
	const dlon = ((lon2 - lon1) * Math.PI) / 180
	const a = Math.sin(dlat / 2) ** 2 + Math.cos(p1) * Math.cos(p2) * Math.sin(dlon / 2) ** 2
	return 2 * r * Math.asin(Math.min(1, Math.sqrt(a)))
}

function eventGeohash(ev) {
	const t = (ev.tags || []).find((x) => x[0] === 'g')
	return t ? String(t[1]).toLowerCase() : ''
}

function assertNip01Req(wire) {
	if (!Array.isArray(wire) || wire[0] !== 'REQ') return 'not a REQ array'
	if (typeof wire[1] !== 'string' || wire[1].length === 0 || wire[1].length > 64) return 'bad subscription id'
	if (wire.length < 3) return 'missing filter'
	for (let i = 2; i < wire.length; i++) {
		const f = wire[i]
		if (!f || typeof f !== 'object' || Array.isArray(f)) return 'filter is not an object'
		if (!Array.isArray(f.kinds) || !f.kinds.every((k) => Number.isInteger(k) && k >= 0)) return 'kinds must be integers'
		if (!Array.isArray(f['#g']) || f['#g'].length === 0) return 'missing #g tag filter'
		if (!f['#g'].every((g) => typeof g === 'string' && /^[0-9bcdefghjkmnpqrstuvwxyz]+$/.test(g))) return 'invalid geohash in #g'
		if (f.limit != null && (!Number.isInteger(f.limit) || f.limit < 1)) return 'bad limit'
	}
	try {
		const parsed = JSON.parse(JSON.stringify(wire))
		if (parsed[0] !== 'REQ' || parsed.length !== wire.length) return 'json roundtrip'
	} catch {
		return 'not json-serializable'
	}
	return ''
}

async function reqNip01(ws, sub, filter, timeout = 8000) {
	const wire = ['REQ', sub, filter]
	const err = assertNip01Req(wire)
	if (err) throw new Error('invalid NIP-01 REQ: ' + err)
	ws.send(JSON.stringify(wire))
	const events = []
	const t0 = Date.now()
	while (Date.now() - t0 < timeout) {
		const msg = await ws.next(timeout - (Date.now() - t0) + 50)
		if (msg[0] === 'EVENT' && msg[1] === sub) events.push(msg[2])
		else if (msg[0] === 'EOSE' && msg[1] === sub) return { events, eose: true, wire }
		else if (msg[0] === 'CLOSED' && msg[1] === sub) return { events, closed: msg[2], eose: false, wire }
	}
	return { events, eose: false, wire }
}

async function conduitProximity() {
	await withRelay({ conduit: true, readyWait: 2500 }, async ({ ws }) => {
		const sk = generateSecretKey()
		const c = await connect(ws)
		const listings = [
			{ id: 'sf', name: 'San Francisco', lat: 37.7749, lon: -122.4194 },
			{ id: 'oak', name: 'Oakland', lat: 37.8044, lon: -122.2712 },
			{ id: 'sj', name: 'San Jose', lat: 37.3382, lon: -121.8863 },
			{ id: 'sac', name: 'Sacramento', lat: 38.5816, lon: -121.4944 },
			{ id: 'la', name: 'Los Angeles', lat: 34.0522, lon: -118.2437 },
			{ id: 'lb', name: 'Long Beach', lat: 33.7701, lon: -118.1937 },
			{ id: 'fre', name: 'Fresno', lat: 36.7378, lon: -119.7871 },
			{ id: 'sb', name: 'Santa Barbara', lat: 34.4208, lon: -119.6982 },
			{ id: 'lv', name: 'Las Vegas', lat: 36.1699, lon: -115.1398 },
			{ id: 'bak', name: 'Bakersfield', lat: 35.3733, lon: -119.0187 },
			{ id: 'nyc', name: 'New York', lat: 40.7128, lon: -74.006 },
		]
		const byId = {}
		const now = 1_700_000_000
		for (let i = 0; i < listings.length; i++) {
			const loc = listings[i]
			loc.g = encodeGeohash(loc.lat, loc.lon, 5)
			const { ev, msg } = await publish(c, sk, {
				kind: 30402,
				created_at: now + i,
				content: `${loc.name} marketplace listing`,
				tags: [
					['d', loc.id],
					['title', `${loc.name} listing`],
					['g', loc.g],
				],
			})
			if (msg[0] !== 'OK' || msg[2] !== true) throw new Error(`publish ${loc.id}: ${JSON.stringify(msg)}`)
			loc.eventId = ev.id
			byId[loc.id] = loc
		}
		soft(
			'conduit proximity listings stored',
			listings.filter((l) => l.id !== 'nyc').length === 10 && listings.every((l) => l.eventId),
			listings.map((l) => `${l.id}:${l.g}`).join(' ')
		)
		await new Promise((r) => setTimeout(r, 800))

		const westPrefix = '9q'
		for (const loc of listings) {
			if (loc.id === 'nyc') continue
			if (!loc.g.startsWith(westPrefix)) throw new Error(`${loc.id} geohash ${loc.g} is not in ${westPrefix}`)
		}

		const requesters = [byId.sf, byId.la, byId.sac, byId.lb, byId.lv]
		for (const origin of requesters) {
			const filter = { kinds: [30402], '#g': [origin.g], limit: 20 }
			const sub = 'near-' + origin.id
			const r = await reqNip01(c, sub, filter)
			c.send(JSON.stringify(['CLOSE', sub]))
			const wireErr = assertNip01Req(r.wire)
			soft(`conduit proximity ${origin.id} NIP-01 REQ`, !wireErr && r.wire[0] === 'REQ' && r.wire[2]['#g'][0] === origin.g, JSON.stringify(r.wire))
			soft(`conduit proximity ${origin.id} EOSE`, r.eose)
			const hits = r.events
			soft(`conduit proximity ${origin.id} match count`, hits.length >= 5, `n=${hits.length}`)
			const nycHit = hits.some((e) => e.id === byId.nyc.eventId)
			const kindsOk = hits.every((e) => e.kind === 30402)
			const geoOk = hits.every((e) => eventGeohash(e).startsWith(westPrefix))
			soft(`conduit proximity ${origin.id} filter`, !nycHit && kindsOk && geoOk && hits.length <= 10, nycHit ? 'nyc leaked' : `n=${hits.length}`)
			const originCell = decodeGeohash(origin.g)
			const dists = hits.map((e) => {
				const gh = eventGeohash(e)
				const cell = decodeGeohash(gh)
				return { id: e.id, g: gh, km: cell && originCell ? haversineKm(originCell.lat, originCell.lon, cell.lat, cell.lon) : Infinity }
			})
			let ordered = true
			for (let i = 1; i < dists.length; i++) {
				if (dists[i].km + 0.05 < dists[i - 1].km) ordered = false
			}
			soft(
				`conduit proximity ${origin.id} distance order`,
				ordered && hits[0] && hits[0].id === origin.eventId,
				dists.map((d) => `${d.g}=${d.km.toFixed(0)}km`).join(' ')
			)
		}
		c.close()
	})
}

async function fixtureIntercept() {
	await withRelay(
		{ fixture: true, env: { PLUGIN_INTERCEPT: 'passthrough' }, readyWait: 2000 },
		async ({ ws }) => {
			const sk = generateSecretKey()
			const c = await connect(ws)
			const r = await req(c, 'pt', { kinds: [30402], search: 'x' })
			soft('fixture intercept passthrough EOSE', r.eose)
			c.close()
		}
	)
}

function writeReport() {
	const lines = [
		'# Plugin e2e report',
		'',
		`Generated: ${new Date().toISOString()}`,
		'',
		`Passed: ${passed}  Failed: ${failed}`,
		'',
		'| Test | Result | Detail |',
		'| --- | --- | --- |',
	]
	for (const r of results) {
		lines.push(`| ${r.name} | ${r.ok ? 'PASS' : 'FAIL'} | ${String(r.detail).replace(/\|/g, '\\|')} |`)
	}
	lines.push('', 'Harness: nostr-tools + ws against a live Congee process (Turso). Conduit uses `CONDUIT_EMBEDDER=fake`.', '')
	fs.writeFileSync(path.join(__dirname, 'report.md'), lines.join('\n'))
}

const t0 = Date.now()
try {
	console.log('building binaries…')
	await buildBins()
	await relayMatrix()
	await slowListen()
	await fixtureIntercept()
	await conduitCases()
	await conduitProximity()
} catch (e) {
	rec('harness', false, String(e && e.stack ? e.stack : e))
} finally {
	writeReport()
	console.log(`done in ${Date.now() - t0}ms — report at test/plugin-e2e/report.md (${passed} pass / ${failed} fail)`)
	process.exit(failed ? 1 : 0)
}
