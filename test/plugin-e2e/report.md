# Plugin e2e report

Generated: 2026-09-20T01:48:02.468Z

Passed: 49  Failed: 0

| Test | Result | Detail |
| --- | --- | --- |
| relay EVENT OK | PASS | dt=2ms |
| relay REQ ids | PASS |  |
| relay REQ authors | PASS |  |
| relay REQ kinds | PASS |  |
| relay REQ tags | PASS |  |
| relay REQ since/until | PASS |  |
| relay REQ limit | PASS |  |
| relay multi-filter | PASS |  |
| relay empty EOSE | PASS |  |
| relay CLOSE | PASS |  |
| relay NIP-50 kind 1 | PASS |  |
| relay bad sig | PASS |  |
| relay invalid JSON | PASS |  |
| slow OnStoredEvent still OK | PASS | dt=3ms baseline=2ms |
| slow listen does not add ~2s | PASS | dt=3 baseline=2 |
| fixture intercept passthrough EOSE | PASS |  |
| conduit store product OK | PASS |  |
| conduit rank bicycle first | PASS | ["30fe50d52be06f66d751e8c7f376eae3f2be3bdce8ed9d452b49c6f1f0829a4f","915f4f671def17dd8de605f2386f82713637c0d08772559cb39ff0a3101dad16"] |
| conduit inactive not delivered | PASS |  |
| conduit geo prefix | PASS |  |
| conduit inject kinds | PASS |  |
| conduit kind 1 search not marketplace | PASS |  |
| conduit limit | PASS |  |
| conduit proximity listings stored | PASS | sf:9q8yy oak:9q9p1 sj:9q9k6 sac:9qce7 la:9q5ct lb:9q5bn fre:9qdbf sb:9q4gu lv:9qqjg bak:9q735 nyc:dr5re |
| conduit proximity sf NIP-01 REQ | PASS | ["REQ","near-sf",{"kinds":[30402],"#g":["9q8yy"],"limit":20}] |
| conduit proximity sf EOSE | PASS |  |
| conduit proximity sf match count | PASS | n=10 |
| conduit proximity sf filter | PASS | n=10 |
| conduit proximity sf distance order | PASS | 9q8yy=0km 9q9p1=13km 9q9k6=67km 9qce7=119km 9qdbf=261km 9q735=404km 9q4gu=445km 9q5ct=560km 9q5bn=585km 9qqjg=667km |
| conduit proximity la NIP-01 REQ | PASS | ["REQ","near-la",{"kinds":[30402],"#g":["9q5ct"],"limit":20}] |
| conduit proximity la EOSE | PASS |  |
| conduit proximity la match count | PASS | n=10 |
| conduit proximity la filter | PASS | n=10 |
| conduit proximity la distance order | PASS | 9q5ct=0km 9q5bn=30km 9q4gu=140km 9q735=163km 9qdbf=329km 9qqjg=368km 9q9k6=493km 9q9p1=556km 9q8yy=560km 9qce7=581km |
| conduit proximity sac NIP-01 REQ | PASS | ["REQ","near-sac",{"kinds":[30402],"#g":["9qce7"],"limit":20}] |
| conduit proximity sac EOSE | PASS |  |
| conduit proximity sac match count | PASS | n=10 |
| conduit proximity sac filter | PASS | n=10 |
| conduit proximity sac distance order | PASS | 9qce7=0km 9q9p1=108km 9q8yy=119km 9q9k6=141km 9qdbf=255km 9q735=418km 9q4gu=487km 9q5ct=581km 9q5bn=609km 9qqjg=618km |
| conduit proximity lb NIP-01 REQ | PASS | ["REQ","near-lb",{"kinds":[30402],"#g":["9q5bn"],"limit":20}] |
| conduit proximity lb EOSE | PASS |  |
| conduit proximity lb match count | PASS | n=10 |
| conduit proximity lb filter | PASS | n=10 |
| conduit proximity lb distance order | PASS | 9q5bn=0km 9q5ct=30km 9q4gu=156km 9q735=192km 9qdbf=358km 9qqjg=385km 9q9k6=518km 9q9p1=581km 9q8yy=585km 9qce7=609km |
| conduit proximity lv NIP-01 REQ | PASS | ["REQ","near-lv",{"kinds":[30402],"#g":["9qqjg"],"limit":20}] |
| conduit proximity lv EOSE | PASS |  |
| conduit proximity lv match count | PASS | n=10 |
| conduit proximity lv filter | PASS | n=10 |
| conduit proximity lv distance order | PASS | 9qqjg=0km 9q735=361km 9q5ct=368km 9q5bn=385km 9qdbf=417km 9q4gu=455km 9q9k6=612km 9qce7=618km 9q9p1=657km 9q8yy=667km |

Harness: nostr-tools + ws against a live Congee process (Turso). Conduit uses `CONDUIT_EMBEDDER=fake`.
