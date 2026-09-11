# Releases

## 2026-09-11 — First deploy
- **What deployed:** https://tailgate.levelbrook.com (Box B, containers `tailgate` + `tailgate-fanoutd`, shared `lb-postgres` db `tailgate_production`).
- **Changed:** initial release. Rails 8 members/follows/activities with a transactional outbox; Go `fanoutd` owning `timeline_entries` with exactly-once fan-out and hybrid push/pull; seeds (61 members, 591 follows, 400 activities).
- **How:** `rsync` to `/root/tailgate`; `docker build` both images on the box; `docker run` on the `kamal` network (300m / 64m caps); `kamal-proxy deploy tailgate --target tailgate:3000 --host tailgate.levelbrook.com --tls`; CF A record `tailgate` -> 5.78.227.227 (DNS-only).
- **Verified:** /, /members, /m/picksqueen, /inspector all 200 over HTTPS; Solid Queue drained 400 outbox events (3,152 timeline rows, 45 pull-mode activities); Go tests 3/3 and Rails tests 7/7 green before build.
