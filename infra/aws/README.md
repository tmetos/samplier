# Samplier — AWS deployment (Terraform)

Provisions the full stack described in `SPECIFICATION.md` §12:

| Resource | Implementation |
|---|---|
| Network | VPC, public/private subnets, NAT gateway, security groups |
| Database | RDS for PostgreSQL (Multi-AZ) with PostGIS |
| API | ECS Fargate service behind an ALB, autoscaling |
| Keycloak | ECS Fargate service, realm imported on boot |
| Frontend | S3 + CloudFront |
| DNS/TLS | Route 53 record + ACM certificate |
| Secrets | Secrets Manager |
| Observability | CloudWatch log groups (14-day retention), Container Insights |
| Alarms | 5xx rate, p95 latency, DB CPU/connections |

## Prerequisites

- Terraform >= 1.6
- AWS credentials with permissions for VPC, RDS, ECS, ECR, ALB, S3, CloudFront,
  Route 53, ACM, Secrets Manager, CloudWatch, IAM
- A registered domain hosted in Route 53 (for the DNS/TLS resources)
- An ECR repository containing the API image (`ghcr.io` mirror or ECR build)

## Usage

```bash
cd infra/aws
cat > terraform.tfvars <<'EOF'
region       = "us-west-2"
project      = "samplier"
environment  = "prod"
domain_name  = "samplier.example.com"
route53_zone_id = "Z0123456789ABCDEFGHIJ"
api_image    = "123456789012.dkr.ecr.us-west-2.amazonaws.com/samplier:latest"
keycloak_image = "quay.io/keycloak/keycloak:latest"

db_instance_class = "db.m5.large"
api_desired_count = 2
api_min_count     = 1
api_max_count     = 4

log_retention_days = 14
EOF

terraform init
terraform plan -out=tfplan
terraform apply tfplan
```

## Remote state

Create the backend bucket and lock table once, then uncomment `backend.tf.example`:

```bash
aws s3api create-bucket --bucket samplier-tfstate --region us-west-2 \
  --create-bucket-configuration LocationConstraint=us-west-2
aws dynamodb create-table --table-name samplier-tflock \
  --attribute-definitions AttributeName=LockID,AttributeType=S \
  --key-schema AttributeName=LockID,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST
```

## Outputs

| Output | Description |
|---|---|
| `api_url` | Public API base URL |
| `ogc_url` | OGC API — Features landing page |
| `keycloak_url` | Keycloak base URL |
| `frontend_url` | CloudFront distribution URL |
| `db_endpoint` | RDS endpoint (private) |
| `db_secret_arn` | Secrets Manager ARN for DB credentials |
