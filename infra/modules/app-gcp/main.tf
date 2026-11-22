# Cloud Run services (API + Keycloak), Cloud CDN-backed frontend bucket,
# global HTTPS load balancer, Cloud Monitoring alerts, and 14-day log retention.

variable "project_id" { type = string }
variable "region" { type = string }
variable "name" { type = string }
variable "network_id" { type = string }
variable "connector_id" { type = string }

variable "api_image" { type = string }
variable "keycloak_image" { type = string }

variable "db_private_ip" { type = string }
variable "db_name" { type = string }
variable "db_secret_id" { type = string }
variable "keycloak_admin_secret_id" { type = string }

variable "domain_name" { type = string }
variable "min_instances" {
  type    = number
  default = 1
}
variable "max_instances" {
  type    = number
  default = 10
}
variable "log_retention_days" {
  type    = number
  default = 14
}
variable "tags" { type = map(string) }

# --- service account -------------------------------------------------------

resource "google_service_account" "api" {
  account_id   = "${var.name}-api"
  display_name = "Samplier API"
  project      = var.project_id
}

resource "google_project_iam_member" "api_secret_accessor" {
  project = var.project_id
  role    = "roles/secretmanager.secretAccessor"
  member  = "serviceAccount:${google_service_account.api.email}"
}

resource "google_project_iam_member" "api_cloudsql_client" {
  project = var.project_id
  role    = "roles/cloudsql.client"
  member  = "serviceAccount:${google_service_account.api.email}"
}

resource "google_project_iam_member" "api_logging" {
  project = var.project_id
  role    = "roles/logging.logWriter"
  member  = "serviceAccount:${google_service_account.api.email}"
}

resource "google_project_iam_member" "api_trace" {
  project = var.project_id
  role    = "roles/cloudtrace.agent"
  member  = "serviceAccount:${google_service_account.api.email}"
}

# --- API Cloud Run service -------------------------------------------------

resource "google_cloud_run_v2_service" "api" {
  name     = "${var.name}-api"
  project  = var.project_id
  location = var.region
  ingress  = "INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER"

  template {
    service_account = google_service_account.api.email

    scaling {
      min_instance_count = var.min_instances
      max_instance_count = var.max_instances
    }

    vpc_access {
      connector = var.connector_id
      egress    = "PRIVATE_RANGES_ONLY"
    }

    containers {
      image = var.api_image

      ports {
        container_port = 8080
      }

      env {
        name  = "PORT"
        value = "8080"
      }
      env {
        name  = "AUTH_ENABLED"
        value = "true"
      }
      env {
        name  = "OIDC_ISSUER"
        value = "https://${var.domain_name}/auth/realms/samplier"
      }
      env {
        name  = "OIDC_CLIENT_ID"
        value = "samplier-ui"
      }
      env {
        name  = "LOG_LEVEL"
        value = "info"
      }
      env {
        name = "DATABASE_URL"
        value_source {
          secret_key_ref {
            secret  = var.db_secret_id
            version = "latest"
          }
        }
      }

      resources {
        limits = {
          cpu    = "2"
          memory = "2Gi"
        }
        cpu_idle = true
      }

      startup_probe {
        http_get {
          path = "/healthz"
          port = 8080
        }
        initial_delay_seconds = 5
        period_seconds        = 10
        failure_threshold     = 6
      }

      liveness_probe {
        http_get {
          path = "/healthz"
          port = 8080
        }
        period_seconds = 30
      }
    }
  }

  labels = var.tags
}

resource "google_cloud_run_v2_service_iam_member" "api_invoker" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.api.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

# --- Keycloak Cloud Run service --------------------------------------------

resource "google_cloud_run_v2_service" "keycloak" {
  name     = "${var.name}-keycloak"
  project  = var.project_id
  location = var.region
  ingress  = "INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER"

  template {
    service_account = google_service_account.api.email

    scaling {
      min_instance_count = 1
      max_instance_count = 2
    }

    vpc_access {
      connector = var.connector_id
      egress    = "PRIVATE_RANGES_ONLY"
    }

    containers {
      image   = var.keycloak_image
      command = ["/opt/keycloak/bin/kc.sh"]
      args    = ["start", "--optimized", "--http-enabled=true", "--proxy-headers=xforwarded"]

      ports {
        container_port = 8080
      }

      env {
        name  = "KC_DB"
        value = "postgres"
      }
      env {
        name  = "KC_DB_URL"
        value = "jdbc:postgresql://${var.db_private_ip}:5432/${var.db_name}"
      }
      env {
        name  = "KC_DB_USERNAME"
        value = "samplier"
      }
      env {
        name  = "KC_HOSTNAME"
        value = "https://${var.domain_name}/auth"
      }
      env {
        name  = "KC_HTTP_RELATIVE_PATH"
        value = "/auth"
      }
      env {
        name  = "KC_HEALTH_ENABLED"
        value = "true"
      }
      env {
        name  = "KC_METRICS_ENABLED"
        value = "true"
      }
      env {
        name = "KC_DB_PASSWORD"
        value_source {
          secret_key_ref {
            secret  = var.db_secret_id
            version = "latest"
          }
        }
      }

      resources {
        limits = {
          cpu    = "2"
          memory = "2Gi"
        }
      }

      startup_probe {
        http_get {
          path = "/auth/health/ready"
          port = 8080
        }
        initial_delay_seconds = 30
        period_seconds        = 10
        failure_threshold     = 12
      }
    }
  }

  labels = var.tags
}

resource "google_cloud_run_v2_service_iam_member" "keycloak_invoker" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.keycloak.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

# --- serverless NEGs and global load balancer ------------------------------

resource "google_compute_region_network_endpoint_group" "api" {
  name                  = "${var.name}-api-neg"
  project               = var.project_id
  region                = var.region
  network_endpoint_type = "SERVERLESS"

  cloud_run {
    service = google_cloud_run_v2_service.api.name
  }
}

resource "google_compute_region_network_endpoint_group" "keycloak" {
  name                  = "${var.name}-keycloak-neg"
  project               = var.project_id
  region                = var.region
  network_endpoint_type = "SERVERLESS"

  cloud_run {
    service = google_cloud_run_v2_service.keycloak.name
  }
}

resource "google_compute_backend_service" "api" {
  name                  = "${var.name}-api-backend"
  project               = var.project_id
  protocol              = "HTTPS"
  load_balancing_scheme = "EXTERNAL_MANAGED"

  backend {
    group = google_compute_region_network_endpoint_group.api.id
  }
}

resource "google_compute_backend_service" "keycloak" {
  name                  = "${var.name}-keycloak-backend"
  project               = var.project_id
  protocol              = "HTTPS"
  load_balancing_scheme = "EXTERNAL_MANAGED"

  backend {
    group = google_compute_region_network_endpoint_group.keycloak.id
  }
}

resource "google_compute_url_map" "this" {
  name            = "${var.name}-urlmap"
  project         = var.project_id
  default_service = google_compute_backend_service.api.id

  host_rule {
    hosts        = [var.domain_name]
    path_matcher = "main"
  }

  path_matcher {
    name            = "main"
    default_service = google_compute_backend_service.api.id

    path_rule {
      paths   = ["/auth", "/auth/*"]
      service = google_compute_backend_service.keycloak.id
    }
  }
}

resource "google_compute_managed_ssl_certificate" "this" {
  name    = "${var.name}-cert"
  project = var.project_id

  managed {
    domains = [var.domain_name]
  }
}

resource "google_compute_target_https_proxy" "this" {
  name             = "${var.name}-https-proxy"
  project          = var.project_id
  url_map          = google_compute_url_map.this.id
  ssl_certificates = [google_compute_managed_ssl_certificate.this.id]
}

resource "google_compute_global_address" "this" {
  name    = "${var.name}-ip"
  project = var.project_id
}

resource "google_compute_global_forwarding_rule" "https" {
  name                  = "${var.name}-https"
  project               = var.project_id
  target                = google_compute_target_https_proxy.this.id
  port_range            = "443"
  ip_address            = google_compute_global_address.this.address
  load_balancing_scheme = "EXTERNAL_MANAGED"
}

# --- frontend bucket + CDN -------------------------------------------------

resource "google_storage_bucket" "frontend" {
  name                        = "${var.project_id}-${var.name}-frontend"
  project                     = var.project_id
  location                    = "US"
  force_destroy               = false
  uniform_bucket_level_access = true

  website {
    main_page_suffix = "index.html"
    not_found_page   = "index.html" # SPA routing
  }

  labels = var.tags
}

resource "google_storage_bucket_iam_member" "frontend_public" {
  bucket = google_storage_bucket.frontend.name
  role   = "roles/storage.objectViewer"
  member = "allUsers"
}

resource "google_compute_backend_bucket" "frontend" {
  name        = "${var.name}-frontend"
  project     = var.project_id
  bucket_name = google_storage_bucket.frontend.name
  enable_cdn  = true

  cdn_policy {
    cache_mode       = "CACHE_ALL_STATIC"
    default_ttl      = 3600
    client_ttl       = 3600
    max_ttl          = 86400
    negative_caching = true
  }
}

# --- log retention (14 days, adjustable) -----------------------------------

resource "google_logging_project_bucket_config" "app" {
  project        = var.project_id
  location       = "global"
  bucket_id      = "${var.name}-logs"
  retention_days = var.log_retention_days
}

resource "google_logging_project_sink" "app" {
  name        = "${var.name}-logs"
  project     = var.project_id
  destination = "logging.googleapis.com/projects/${var.project_id}/locations/global/buckets/${google_logging_project_bucket_config.app.bucket_id}"

  filter = <<-EOT
    resource.type="cloud_run_revision"
    resource.labels.service_name=~"^${var.name}-(api|keycloak)$"
  EOT

  unique_writer_identity = true
}

resource "google_logging_project_bucket_config" "default" {
  project        = var.project_id
  location       = "global"
  bucket_id      = "_Default"
  retention_days = var.log_retention_days
}

# --- alerts ----------------------------------------------------------------

resource "google_monitoring_alert_policy" "api_5xx" {
  display_name = "${var.name} API 5xx rate"
  project      = var.project_id
  combiner     = "OR"

  conditions {
    display_name = "5xx ratio > 5%"
    condition_threshold {
      filter = join(" AND ", [
        "resource.type=\"cloud_run_revision\"",
        "resource.labels.service_name=\"${var.name}-api\"",
        "metric.type=\"run.googleapis.com/request_count\"",
        "metric.labels.response_code_class=\"5xx\"",
      ])
      comparison      = "COMPARISON_GT"
      threshold_value = 10
      duration        = "300s"

      aggregations {
        alignment_period   = "300s"
        per_series_aligner = "ALIGN_RATE"
      }
    }
  }

  alert_strategy {
    auto_close = "1800s"
  }
}

resource "google_monitoring_alert_policy" "api_latency" {
  display_name = "${var.name} API p95 latency"
  project      = var.project_id
  combiner     = "OR"

  conditions {
    display_name = "p95 > 2s"
    condition_threshold {
      filter = join(" AND ", [
        "resource.type=\"cloud_run_revision\"",
        "resource.labels.service_name=\"${var.name}-api\"",
        "metric.type=\"run.googleapis.com/request_latencies\"",
      ])
      comparison      = "COMPARISON_GT"
      threshold_value = 2000
      duration        = "300s"

      aggregations {
        alignment_period   = "300s"
        per_series_aligner = "ALIGN_PERCENTILE_95"
      }
    }
  }
}

resource "google_monitoring_alert_policy" "sql_cpu" {
  display_name = "${var.name} Cloud SQL CPU"
  project      = var.project_id
  combiner     = "OR"

  conditions {
    display_name = "CPU > 80%"
    condition_threshold {
      filter          = "resource.type=\"cloudsql_database\" AND metric.type=\"cloudsql.googleapis.com/database/cpu/utilization\""
      comparison      = "COMPARISON_GT"
      threshold_value = 0.8
      duration        = "600s"

      aggregations {
        alignment_period   = "300s"
        per_series_aligner = "ALIGN_MEAN"
      }
    }
  }
}

output "api_url" { value = google_cloud_run_v2_service.api.uri }
output "keycloak_url" { value = google_cloud_run_v2_service.keycloak.uri }
output "frontend_bucket" { value = google_storage_bucket.frontend.name }
output "load_balancer_ip" { value = google_compute_global_address.this.address }
output "log_bucket" { value = google_logging_project_bucket_config.app.bucket_id }
