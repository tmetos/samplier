output "api_url" {
  description = "Public API base URL"
  value       = "https://${var.domain_name}"
}

output "ogc_url" {
  description = "OGC API — Features landing page"
  value       = "https://${var.domain_name}/ogc/"
}

output "keycloak_url" {
  description = "Keycloak base URL"
  value       = "https://${var.domain_name}/auth"
}

output "alb_dns_name" {
  description = "ALB DNS name (pre-DNS testing)"
  value       = module.app.alb_dns_name
}

output "db_endpoint" {
  description = "RDS endpoint (private; reachable from the VPC only)"
  value       = module.datastore.endpoint
}

output "db_secret_arn" {
  description = "Secrets Manager ARN holding database credentials"
  value       = module.datastore.secret_arn
}

output "certificate_arn" {
  description = "Validated ACM certificate ARN"
  value       = aws_acm_certificate_validation.this.certificate_arn
}

output "vpc_id" {
  description = "VPC id"
  value       = module.network.vpc_id
}
