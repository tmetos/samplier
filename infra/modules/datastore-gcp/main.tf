# Cloud SQL for PostgreSQL with PostGIS, HA, private IP, and Secret Manager.

variable "project_id" { type = string }
variable "region" { type = string }
variable "name" { type = string }
variable "network_id" { type = string }
variable "tier" {
  type    = string
  default = "db-custom-4-8192"
}
variable "availability_type" {
  type    = string
  default = "REGIONAL"
}
variable "disk_size_gb" {
  type    = number
  default = 100
}
variable "backup_retention_days" {
  type    = number
  default = 14
}
variable "tags" { type = map(string) }

resource "random_password" "db" {
  length  = 32
  special = false
}

resource "random_password" "keycloak_admin" {
  length  = 32
  special = false
}

resource "google_sql_database_instance" "this" {
  name             = "${var.name}-db"
  project          = var.project_id
  region           = var.region
  database_version = "POSTGRES_16"

  deletion_protection = true

  settings {
    tier              = var.tier
    availability_type = var.availability_type
    disk_size         = var.disk_size_gb
    disk_autoresize   = true
    disk_type         = "PD_SSD"

    ip_configuration {
      ipv4_enabled    = false
      private_network = var.network_id
      ssl_mode        = "ENCRYPTED_ONLY"
    }

    backup_configuration {
      enabled                        = true
      start_time                     = "07:00"
      point_in_time_recovery_enabled = true
      transaction_log_retention_days = 7

      backup_retention_settings {
        retained_backups = var.backup_retention_days
        retention_unit   = "COUNT"
      }
    }

    maintenance_window {
      day          = 7
      hour         = 8
      update_track = "stable"
    }

    insights_config {
      query_insights_enabled  = true
      query_string_length     = 1024
      record_application_tags = true
      record_client_address   = true
    }

    database_flags {
      name  = "log_min_duration_statement"
      value = "1000"
    }

    # PostGIS is installed by schema.sql (CREATE EXTENSION postgis).
    user_labels = var.tags
  }

  depends_on = []
}

resource "google_sql_database" "this" {
  name     = "samplier"
  project  = var.project_id
  instance = google_sql_database_instance.this.name
}

resource "google_sql_user" "this" {
  name     = "samplier"
  project  = var.project_id
  instance = google_sql_database_instance.this.name
  password = random_password.db.result
}

resource "google_secret_manager_secret" "db" {
  secret_id = "${var.name}-db"
  project   = var.project_id

  replication {
    auto {}
  }

  labels = var.tags
}

resource "google_secret_manager_secret_version" "db" {
  secret = google_secret_manager_secret.db.id
  secret_data = jsonencode({
    username = google_sql_user.this.name
    password = random_password.db.result
    host     = google_sql_database_instance.this.private_ip_address
    port     = 5432
    dbname   = google_sql_database.this.name
    url      = "postgres://${google_sql_user.this.name}:${random_password.db.result}@${google_sql_database_instance.this.private_ip_address}:5432/${google_sql_database.this.name}?sslmode=require"
  })
}

resource "google_secret_manager_secret" "keycloak_admin" {
  secret_id = "${var.name}-keycloak-admin"
  project   = var.project_id

  replication {
    auto {}
  }

  labels = var.tags
}

resource "google_secret_manager_secret_version" "keycloak_admin" {
  secret = google_secret_manager_secret.keycloak_admin.id
  secret_data = jsonencode({
    username = "admin"
    password = random_password.keycloak_admin.result
  })
}

output "instance_name" { value = google_sql_database_instance.this.name }
output "private_ip" { value = google_sql_database_instance.this.private_ip_address }
output "database_name" { value = google_sql_database.this.name }
output "db_secret_id" { value = google_secret_manager_secret.db.secret_id }
output "keycloak_admin_secret_id" { value = google_secret_manager_secret.keycloak_admin.secret_id }
