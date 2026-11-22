# Samplier — GCP deployment (Terraform)

Provisions the stack from `SPECIFICATION.md` §12:

| Resource | Implementation |
|---|---|
| Network | VPC, private subnet, Serverless VPC Access connector |
| Database | Cloud SQL PostgreSQL 16 (HA, private IP, PostGIS) |
| API | Cloud Run (min 1 / max 10 instances) |
| Keycloak | Cloud Run, realm imported on boot |
| Frontend | Cloud Storage + Cloud CDN |
| Ingress | Global external HTTPS load balancer + managed certificate |
| Secrets | Secret Manager |
| DNS | Cloud DNS record (set `manage_dns = true`) |
| Observability | Cloud Logging (14-day bucket), Cloud Monitoring alerts |

## Prerequisites

- Terraform >= 1.6
- `gcloud auth application-default login`
- A GCP project with billing enabled and the following APIs enabled:
  `compute`, `run`, `sqladmin`, `secretmanager`, `vpcaccess`, `logging`,
  `monitoring`, `storage`, `dns`
- A global static IP or a domain you control

## Usage

```bash
cd infra/gcp
cat > terraform.tfvars <<'EOF'
project_id  = "my-samplier-project"
region      = "us-west1"
domain_name = "samplier.example.com"
dns_zone_name = "example-com"

api_image      = "us-west1-docker.pkg.dev/my-samplier-project/samplier/api:latest"
keycloak_image = "quay.io/keycloak/keycloak:latest"

sql_tier          = "db-custom-4-8192"
sql_disk_size_gb  = 100
min_instances     = 1
max_instances     = 10
log_retention_days = 14
EOF

terraform init
terraform plan -out=tfplan
terraform apply tfplan
```

## Post-deploy

1. Apply the schema and seed to Cloud SQL (private IP, via the connector or
   Cloud SQL Auth Proxy):
   ```bash
   cloud-sql-proxy --private-ip PROJECT:REGION:samplier-db &
   psql "host=127.0.0.1 user=samplier dbname=samplier" -f schema.sql
   psql "host=127.0.0.1 user=samplier dbname=samplier" -f seed.sql
   ```
2. Create the `samplier` realm in Keycloak and a `samplier-ui` client with
   redirect URIs for your frontend.
3. Upload the built frontend bundle to the frontend bucket, then invalidate CDN.

## Outputs

| Output | Description |
|---|---|
| `load_balancer_ip` | Point DNS here |
| `api_url` | Cloud Run API URL (direct, bypassing the LB) |
| `keycloak_url` | Cloud Run Keycloak URL |
| `frontend_bucket` | Bucket for the React bundle |
| `log_bucket` | Cloud Logging bucket (14-day retention) |
