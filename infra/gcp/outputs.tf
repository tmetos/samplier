output "load_balancer_ip" {
  description = "Global load balancer IP (point DNS here)"
  value       = module.app.load_balancer_ip
}

output "api_url" {
  description = "Cloud Run API URL"
  value       = module.app.api_url
}

output "keycloak_url" {
  description = "Cloud Run Keycloak URL"
  value       = module.app.keycloak_url
}

output "frontend_bucket" {
  description = "GCS bucket for the React bundle"
  value       = module.app.frontend_bucket
}

output "log_bucket" {
  description = "Cloud Logging bucket id (retention from var.log_retention_days)"
  value       = module.app.log_bucket
}

output "db_private_ip" {
  description = "Cloud SQL private IP"
  value       = module.datastore.private_ip
}

output "db_secret_id" {
  description = "Secret Manager id holding database credentials"
  value       = module.datastore.db_secret_id
}
