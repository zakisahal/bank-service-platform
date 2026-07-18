# Decisions and Trade-offs

## Biggest decisions

**Messaging primitive: Redis Streams, not Kafka/SQS/NATS.** One consumer group doing audit logging doesn't justify Kafka's ZooKeeper/KRaft operational cost, and SQS isn't runnable locally without LocalStack. Redis Streams gives consumer groups, a per-message delivery counter, and `XCLAIM`-based redelivery natively — everything idempotency and poison-message handling need — for one container's worth of ops burden. Trade-off: no partition-level ordering and no long-retention replay story. If this grows into a system-of-record for events rather than a queue, Kafka becomes the right call.

**Connection pooling: none yet, `pgxpool` per pod only.** At KEDA's max of 6 API replicas plus a few consumer replicas, total connections stay well inside a `db.t4g.micro`'s 100-connection ceiling without a pooler. Trade-off: this expires — more replicas or more services sharing the database hit RDS's connection ceiling regardless of per-pod pooling. The threshold I'd watch: total possible connections approaching ~60% of `max_connections`. Past that, RDS Proxy in **transaction**-pooling mode (not session — neither DB role uses session-level state) is the fix; not sized here without real traffic data.

**Autoscaling signal: KEDA on in-flight request concurrency, not CPU.** This service is I/O-bound (blocked on Postgres round-trips), so a pod can be fully saturated at 15% CPU — CPU would scale out after the service was already degraded. Trade-off: the threshold (avg 10 in-flight requests/pod) is reasoned from the workload shape, not load-tested — no KEDA operator was installed on the validation cluster.

## SLO and alerting

**SLI**: proportion of non-5xx responses to `GET /api/accounts/{id}`. **Target**: 99.9% over a rolling 30-day window. **Budget**: 0.1% ≈ 43 minutes/month.

Derived from what a single-region, Multi-AZ-RDS, multi-replica architecture can actually back up, not from what's easy to measure. A failed balance lookup reads as "is my money okay" to a user, which argues for stricter — but promising 99.99% without a proven failover runbook (see Recovery) is a number I couldn't defend in an incident review.

Alerting is **multi-window, multi-burn-rate**, not a single static threshold (`observability/alerts.yaml`):
- **Fast burn** (page): error rate > 14.4× the SLO's threshold, sustained over *both* a 1h and a 5m window — the 30-day budget would be gone in ~2 days at that rate.
- **Slow burn** (ticket): error rate > 1× the threshold, sustained over *both* a 3-day and a 6h window — catches a persistent 0.1-0.2% error rate that never spikes enough to trip a "5% for 5 minutes" rule but still burns the budget too fast.

Two windows per rule so a five-minute blip can't page alone — only sustained burn does.

**If I could keep only one alert, it's the fast-burn rule, not `db_up == 0`.** `db_up` is cause-level — it only catches Postgres-unreachable and misses a code regression or slow-query pileup that degrades users without failing a DB ping. The fast-burn alert is symptom-level: it fires on user-facing impact regardless of cause, and a DB outage is a special case of it (100% errors burns budget fastest, tripping this almost immediately anyway). Symptom-based paging is the more defensible default.

## Least privilege and blast radius

**API pod RCE**: inherits `bank_service_api`'s grants — `SELECT` on `accounts` only. Can read every balance and owner name (a real, stated exposure — that's this role's whole job), but cannot write to `accounts`, cannot touch `processed_events`/`audit_log` at all, cannot run DDL, isn't the table owner. NetworkPolicy egress is DNS + Postgres:5432 only — no internet, no lateral movement to other pods. IRSA scopes `secretsmanager:GetSecretValue` to exactly one ARN (its own credential) — can't enumerate or read any other secret, can't touch IAM/EKS/RDS control-plane APIs.

**Consumer pod RCE**: `bank_service_consumer` can `INSERT` into `audit_log`, `SELECT`/`INSERT` on `processed_events`, nothing else — no access to `accounts` at all. Real residual risk worth naming: it *can* insert forged audit rows (that's its job), so a compromised consumer could poison the audit trail with fabricated entries — but cannot alter or delete real rows already there (no `UPDATE`/`DELETE`), so forgeries are additions, not replacements, and are themselves evidence of compromise. Egress is Postgres + Redis + DNS only.

**Denied by default in both**: the schema's `PUBLIC` grant is revoked; neither role owns a table or is superuser; NetworkPolicy is default-deny with explicit allows only.

## Recovery

**Postgres — RPO ~5 min, RTO ~30 min (bad data/logical corruption); RTO ~1-2 min (AZ failure).** RPO is bounded by RDS PITR's continuous log replication, not the daily snapshot window. Logical-recovery RTO assumes restoring the latest PITR snapshot to a new instance plus endpoint/Secrets Manager cutover; AZ-failure RTO is Multi-AZ's automatic failover, which doesn't involve a restore at all. These are different failure modes with different numbers on purpose.

**Proving the runbook works, not asserting it**: a scheduled monthly job — restore the latest backup to a scratch RDS instance in an isolated VPC, run a smoke-test query against a known baseline, tear it down, page if the restore fails or exceeds a time budget. Turns "we have backups" into a continuously-verified claim. **Not built here** (no RDS exists in this exercise) — it's first on my list for a real rollout.

**Messaging tier during/after an outage**: if a consumer crashes mid-processing, its claimed-but-unacked message stays in the group's pending list; `reclaimStale` reclaims it via `XCLAIM` once another replica is up — delayed, not lost. If every consumer is down, new events keep landing via `XADD` as long as Redis is reachable; they queue unread and drain automatically once a consumer returns. Poison messages hit during backlog drain use the same `maxDeliveries`-then-DLQ path as normal operation — no special-casing needed. The real gap: if Redis itself is unreachable, producer-side `XADD` fails or blocks, and this repo doesn't implement producer-side retry/buffering for that — out of scope per "don't over-invest in the broker," but a genuine gap in the full outage story, not a hidden one.

## What I cut

Full list and reasoning in DECISIONS.md; build order for a real rollout:

1. **Backup-restore game day automation** — recovery is currently reasoning, not a proven capability, and that's the gap that hurts most in an actual incident.
2. **IAM permissions boundary on `terraform_apply`** — cheap, closes a real privilege-escalation shape the ARN-pattern scoping doesn't fully close.
3. **Live Argo CD** — the CD pipeline's GitOps handoff is a documented boundary, not a working reconciler yet.
4. **Load-testing the KEDA threshold** — the signal is right in principle; the number is reasoned, not measured.
