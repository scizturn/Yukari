# Yukari Reader / Makoto Sender Split Plan

**Goal:** Split birthday email automation into Yukari as the Hanayo DB reader and Makoto as the Redis-consuming sender.

**Architecture:** Yukari reads birthday users and personalization from Hanayo DB, builds complete job JSON, and pushes to Redis. Makoto consumes Redis jobs, generates vouchers, sends Kirim.email, and logs to Discord. Redis is temporary queue state; Discord is operational logging.

**No commits or pushes:** All files remain uncommitted until explicitly requested.
