# bank-service platform

A regulated-bank-shaped platform around one small Go HTTP service and its
event consumer: containers, a Helm chart, Terraform for the AWS layer this
would run on, CI/CD, and the data/messaging/security/observability controls
around all of it.

See [DECISIONS.md](DECISIONS.md) for the reasoning, the SLO this is built
around, the local→AWS mapping, and what was deliberately left out.

## Repo layout

```
service/            Go HTTP API + event consumer + migrations + Dockerfile + docker-compose
  cmd/api/            the HTTP service (GET /healthz /readyz /metrics /api/accounts/{id})
  cmd/consumer/        the idempotent Redis Streams consumer
  internal/events/     consumer logic: dedupe, DLQ, poison-message handling
  internal/tracing/    OpenTelemetry wiring
  migrations/          0001 schema+seed, 0002 event tables, 0003 least-privilege roles
deploy/helm/         Helm chart for both the service and the consumer
infra/terraform/      VPC/EKS/ECR/RDS/IAM/Secrets Manager, authored + validated (not applied)
  envs/dev/plan-output.txt   committed, sanitised `terraform plan` output
.github/workflows/    CI (test/scan/lint/validate) and CD (OIDC→ECR, GitOps handoff)
observability/        Prometheus alerting rules + a Grafana dashboard-as-code panel
```

## Prerequisites

Go 1.25+, Docker, Helm 3.19+, kubectl, a local Kubernetes cluster (minikube
used below; kind/k3d work the same way), Terraform 1.5+, `redis-cli` if you
want to poke the stream by hand (or just `docker exec` into the redis
container, as below).

## 1. Fastest path: docker compose

```sh
cd service
docker compose up --build
# in another shell:
curl localhost:8080/healthz
curl localhost:8080/readyz
curl localhost:8080/api/accounts/acc_1001
curl localhost:8080/metrics
```

This brings up Postgres (with the schema, seed data, and both
least-privilege roles applied via `local-init/`), Redis, the API (connected
as `bank_service_api`), and the consumer (connected as
`bank_service_consumer`).

> This environment's Docker install didn't have the `docker compose`
> plugin, so everything above was validated by running the equivalent
> `docker network create` / `docker run` commands by hand instead (see
> DECISIONS.md for the exact sequence used, including the least-privilege
> and messaging verification below). The compose file itself is standard
> v2 syntax and should work as-is anywhere with Docker Compose installed.

Run the unit tests directly (no containers needed for most of them; the
consumer's idempotency test needs a real Postgres and is skipped without
one):

```sh
cd service
go vet ./...
go test ./... -race -cover
```

### Verifying the least-privilege roles

```sh
# SELECT works, everything else is denied:
docker exec -e PGPASSWORD=bank-api-demo-password bank-postgres \
  psql -U bank_service_api -d bank -c "SELECT id, owner FROM accounts LIMIT 1;"
docker exec -e PGPASSWORD=bank-api-demo-password bank-postgres \
  psql -U bank_service_api -d bank -c "DROP TABLE accounts;"   # ERROR: must be owner of table accounts

# consumer role can write the audit trail but never read accounts or delete its own rows:
docker exec -e PGPASSWORD=bank-consumer-demo-password bank-postgres \
  psql -U bank_service_consumer -d bank -c "SELECT * FROM accounts;"     # ERROR: permission denied
docker exec -e PGPASSWORD=bank-consumer-demo-password bank-postgres \
  psql -U bank_service_consumer -d bank -c "DELETE FROM audit_log;"      # ERROR: permission denied
```

### Verifying the messaging tier (idempotency + poison messages)

```sh
# Publish the same event twice - simulates at-least-once redelivery:
docker exec bank-redis redis-cli XADD bank.events '*' data \
  '{"id":"evt-001","account_id":"acc_1001","type":"account.viewed","payload":{},"occurred_at":"2026-01-01T00:00:00Z"}'
docker exec bank-redis redis-cli XADD bank.events '*' data \
  '{"id":"evt-001","account_id":"acc_1001","type":"account.viewed","payload":{},"occurred_at":"2026-01-01T00:00:00Z"}'
# audit_log should have exactly 1 row for evt-001:
docker exec bank-postgres psql -U bank -d bank -c "SELECT count(*) FROM audit_log WHERE event_id='evt-001';"

# Publish a malformed event (no "id") - should be dead-lettered immediately:
docker exec bank-redis redis-cli XADD bank.events '*' data '{"account_id":"acc_1001","type":"broken"}'
docker exec bank-redis redis-cli XRANGE bank.events.dlq - +
docker exec bank-redis redis-cli XPENDING bank.events audit-log   # back to 0 pending
```

Both of the above were run against a live stack while building this repo -
see DECISIONS.md "Messaging tier" for what was observed.

### Verifying the container hardening

```sh
docker build --target api -t bank-service:local service
docker run -d --name bank-service -p 8080:8080 -e DATABASE_URL=... bank-service:local
docker ps --filter name=bank-service   # STATUS shows "(healthy)" once HEALTHCHECK passes
```

Distroless has no shell, so `HEALTHCHECK` runs `/bank-service -healthcheck`
directly (JSON exec form, no shell involved) rather than shelling out to
`curl`.

## 2. Live Kubernetes: minikube

This was run end-to-end against a real minikube cluster, including a
zero-downtime rolling restart, as part of building this repo.

```sh
minikube start
eval $(minikube docker-env)          # build straight into minikube's daemon
docker build --target api -t bank-service:local service
docker build --target consumer -t bank-consumer:local service
eval $(minikube docker-env -u)

kubectl create namespace bank-demo
helm install bank-service deploy/helm/bank-service \
  -n bank-demo \
  --set postgres.enabled=true \      # bundled dev-only Postgres; prod points at RDS instead
  --set redis.enabled=true \         # bundled dev-only Redis; prod points at a managed broker
  --set consumer.enabled=true \
  --wait --timeout 3m

kubectl get pods -n bank-demo
kubectl port-forward svc/bank-service 8080:80 -n bank-demo &
curl localhost:8080/healthz
curl localhost:8080/readyz
curl localhost:8080/api/accounts/acc_1001
curl localhost:8080/metrics

# Prove the drain/rollout behavior:
kubectl rollout restart deployment/bank-service -n bank-demo
kubectl rollout status deployment/bank-service -n bank-demo
```

`helm lint` and `helm template` (default values, the
`postgres.enabled=true,redis.enabled=true,consumer.enabled=true,monitoring.serviceMonitor.enabled=true`
combination, the `autoscaling.strategy=hpa` fallback, and
`ingress.enabled=true`) all run in CI (`.github/workflows/ci.yaml`) on
every PR.

By default the chart autoscales with **KEDA** on in-flight request
concurrency rather than CPU (see DECISIONS.md "Autoscaling metric" for why)
- that requires the KEDA operator in-cluster, which this repo doesn't
install. Set `--set autoscaling.strategy=hpa` for a CPU-based
`HorizontalPodAutoscaler` on a cluster without KEDA.

Teardown:

```sh
helm uninstall bank-service -n bank-demo
kubectl delete namespace bank-demo
minikube stop
```

## 3. Terraform (author + validate + plan only — no apply)

Per the brief, this repo does not run against a real AWS account.

```sh
cd infra/terraform/envs/dev
terraform fmt -check -recursive ../../..
terraform init
terraform validate
terraform plan   # succeeds with no real credentials - see providers.tf and DECISIONS.md
```

The sanitised output of that `terraform plan` is committed at
[`infra/terraform/envs/dev/plan-output.txt`](infra/terraform/envs/dev/plan-output.txt)
(46 resources to add, 0 errors, run with `enable_github_oidc=true` so all
three CI/CD IAM roles below are actually in the plan: VPC, EKS + node group
+ OIDC provider, ECR, RDS, KMS, Secrets Manager, IRSA + the three
GitHub-OIDC roles). Copy `terraform.tfvars.example` → `terraform.tfvars`
and fill in a real AWS account before ever running `terraform apply`.

**No static AWS credentials exist anywhere in this repo, including for
Terraform itself** - three separate GitHub Actions OIDC roles
(`infra/terraform/modules/iam`), each scoped to one workflow and nothing
more:

| Role | Used by | Can do |
|---|---|---|
| `terraform_plan` | `ci.yaml`, every PR | Read-only AWS access, no secret values |
| `terraform_apply` | `terraform-apply.yaml`, manual + environment-gated | Full CRUD on this stack's resources, IAM scoped by name pattern |
| `ci_deploy` | `cd.yaml`, on merge to main | Push images to one ECR repo only |

See DECISIONS.md "CI/CD and deployment strategy" for the full trust-policy
`sub` conditions and the residual IAM privilege-escalation risk the
`terraform_apply` role's scoping doesn't fully close.

## 4. CI/CD

`.github/workflows/ci.yaml` runs on every PR: Go unit tests against a real
Postgres service container with the actual least-privilege roles applied
(including a DB-backed idempotency test for the consumer), `govulncheck`, a
Trivy scan per image (API and consumer, `exit-code: 1` - fails the check
and blocks merge once this check is marked required in branch protection;
that's a one-time repo setting this take-home doesn't have a remote to
configure), Helm lint/template, Terraform fmt/validate/**plan**, and a
gitleaks secret scan. The Terraform `plan` step authenticates via the
`terraform_plan` OIDC role and is conditional on `TF_PLAN_ROLE_ARN` being
configured as a repo variable, so this workflow stays green in a fresh
clone before that role exists in a real account, and runs for real once it
does.

`.github/workflows/cd.yaml` authenticates to AWS via **GitHub OIDC** (no
static AWS keys anywhere), pushes immutably-tagged images
(`api-v<run>-<sha>` / `consumer-v<run>-<sha>`, never `latest`) to ECR, and
gates on a manual approval (the `production` GitHub Environment) before the
deploy step - which itself documents (rather than executes) the GitOps
handoff. `.github/workflows/terraform-apply.yaml` is the equivalent for
infrastructure changes: manual `workflow_dispatch` only, gated by a
*separate* `terraform-apply` Environment so an app-deploy approver and an
infrastructure-change approver don't have to be the same authorization.
See DECISIONS.md "CI/CD and deployment strategy" for the full reasoning and
the two application rollback paths (`helm rollback` vs. `git revert`).

None of these three workflows has been executed by GitHub Actions as part
of this submission (no GitHub remote is wired up yet, and no AWS account
exists behind any of the three OIDC roles); all were validated for
correct YAML locally, and the CI job's exact migration+test sequence
(dummy role passwords, `TEST_DATABASE_URL`/`ADMIN_TEST_DATABASE_URL`) was
run locally against a real Postgres before being put in the workflow.

## 5. Observability

`observability/alerts.yaml` is a `PrometheusRule` (error rate, p99 latency,
`db_up`, crash-looping, zero-replicas-available) for a cluster running
kube-prometheus-stack - see DECISIONS.md for which one would actually page
and why. `observability/dashboard.json` is a panel-as-code Grafana
dashboard (request rate, error rate, p95 latency). The chart's
`ServiceMonitor` (off by default, see `deploy/helm/bank-service/values.yaml`)
is what feeds Prometheus from a live cluster.

Tracing defaults to a stdout exporter (`internal/tracing`), so a real trace
path (HTTP request span → nested Postgres-query span) can be observed with
zero extra infrastructure - see DECISIONS.md "Tracing" for what that looked
like when run. Set `OTEL_EXPORTER_OTLP_ENDPOINT` (or the chart's
`tracing.otlpEndpoint` value) to export to a real collector instead.
