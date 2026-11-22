variable "region" {
  description = "AWS region"
  type        = string
  default     = "us-west-2"
}

variable "project" {
  description = "Project name used as a resource prefix"
  type        = string
  default     = "samplier"
}

variable "environment" {
  description = "Deployment environment (dev, staging, prod)"
  type        = string
  default     = "prod"
}

variable "vpc_cidr" {
  description = "CIDR block for the VPC"
  type        = string
  default     = "10.42.0.0/16"
}

variable "domain_name" {
  description = "Public hostname for the API (must be in the Route 53 zone below)"
  type        = string
}

variable "route53_zone_id" {
  description = "Route 53 hosted zone id for domain_name"
  type        = string
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

variable "db_instance_class" {
  description = "RDS instance class"
  type        = string
  default     = "db.m5.large"
}

variable "db_allocated_storage" {
  description = "Initial RDS storage in GiB"
  type        = number
  default     = 50
}

variable "db_max_allocated_storage" {
  description = "Autoscaling storage ceiling in GiB"
  type        = number
  default     = 500
}

variable "db_multi_az" {
  description = "Enable Multi-AZ for the database"
  type        = bool
  default     = true
}

variable "db_backup_retention_days" {
  description = "RDS automated backup retention in days"
  type        = number
  default     = 14
}

variable "db_max_connections_alarm" {
  description = "Connection-count threshold for the RDS alarm"
  type        = number
  default     = 150
}

# --- compute ---------------------------------------------------------------

variable "api_cpu" {
  description = "Fargate task CPU units for the API"
  type        = number
  default     = 1024
}

variable "api_memory" {
  description = "Fargate task memory (MiB) for the API"
  type        = number
  default     = 2048
}

variable "api_desired_count" {
  description = "Desired number of API tasks"
  type        = number
  default     = 2
}

variable "api_min_count" {
  description = "Minimum API tasks for autoscaling"
  type        = number
  default     = 1
}

variable "api_max_count" {
  description = "Maximum API tasks for autoscaling"
  type        = number
  default     = 4
}

# --- observability ---------------------------------------------------------

variable "log_retention_days" {
  description = "CloudWatch log retention in days (spec default: 14, adjustable)"
  type        = number
  default     = 14
}
