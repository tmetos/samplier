variable "project_id" {
  description = "GCP project id"
  type        = string
}

variable "region" {
  description = "GCP region"
  type        = string
  default     = "us-west1"
}

variable "project" {
  description = "Resource name prefix"
  type        = string
  default     = "samplier"
}

variable "environment" {
  description = "Deployment environment"
  type        = string
  default     = "prod"
}

variable "domain_name" {
  description = "Public hostname routed to the load balancer"
  type        = string
}

variable "dns_zone_name" {
  description = "Cloud DNS managed zone name; leave empty to skip DNS management"
  type        = string
  default     = ""
}

variable "api_image" {
  description = "Container image for the Samplier API"
  type        = string
}

variable "keycloak_image" {
  description = "Container image for Keycloak"
  type        = string
  default     = "quay.io/keycloak/keycloak:latest"
}

# --- database --------------------------------------------------------------

variable "sql_tier" {
  description = "Cloud SQL machine tier (dev: db-f1-micro, prod: db-custom-4-8192)"
  type        = string
  default     = "db-custom-4-8192"
}

variable "sql_availability_type" {
  description = "REGIONAL (HA) or ZONAL"
  type        = string
  default     = "REGIONAL"
}

variable "sql_disk_size_gb" {
  description = "Cloud SQL disk size in GiB"
  type        = number
  default     = 100
}

variable "sql_backup_retention_days" {
  description = "Number of automated backups retained"
  type        = number
  default     = 14
}

# --- compute ---------------------------------------------------------------

variable "min_instances" {
  description = "Minimum Cloud Run API instances"
  type        = number
  default     = 1
}

variable "max_instances" {
  description = "Maximum Cloud Run API instances"
  type        = number
  default     = 10
}

# --- observability ---------------------------------------------------------

variable "log_retention_days" {
  description = "Cloud Logging retention in days (spec default: 14, adjustable)"
  type        = number
  default     = 14
}
