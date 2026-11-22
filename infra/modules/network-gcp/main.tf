# VPC, subnets, serverless VPC connector, and firewall rules.

resource "google_compute_network" "this" {
  name                    = "${var.name}-vpc"
  project                 = var.project_id
  auto_create_subnetworks = false
  routing_mode            = "REGIONAL"
}

resource "google_compute_subnetwork" "private" {
  name                     = "${var.name}-private"
  project                  = var.project_id
  region                   = var.region
  network                  = google_compute_network.this.id
  ip_cidr_range            = "10.50.0.0/20"
  private_ip_google_access = true

  log_config {
    aggregation_interval = "INTERVAL_10_MIN"
    flow_sampling        = 0.5
    metadata             = "INCLUDE_ALL_METADATA"
  }
}

# Serverless VPC Access connector so Cloud Run can reach Cloud SQL privately.
resource "google_vpc_access_connector" "this" {
  name          = "${var.name}-conn"
  project       = var.project_id
  region        = var.region
  network       = google_compute_network.this.id
  ip_cidr_range = "10.50.16.0/28"
  min_instances = 2
  max_instances = 10

  depends_on = [google_compute_subnetwork.private]
}

# Allow health checks from Google's load balancer ranges.
resource "google_compute_firewall" "allow_lb" {
  name      = "${var.name}-allow-lb"
  project   = var.project_id
  network   = google_compute_network.this.name
  direction = "INGRESS"

  source_ranges = ["130.211.0.0/22", "35.191.0.0/16"]

  allow {
    protocol = "tcp"
    ports    = ["8080", "8443"]
  }
}

resource "google_compute_firewall" "allow_internal" {
  name      = "${var.name}-allow-internal"
  project   = var.project_id
  network   = google_compute_network.this.name
  direction = "INGRESS"

  source_ranges = ["10.50.0.0/20", "10.50.16.0/28"]

  allow {
    protocol = "tcp"
    ports    = ["8080", "5432"]
  }
}

output "network_id" { value = google_compute_network.this.id }
output "network_name" { value = google_compute_network.this.name }
output "private_subnet_id" { value = google_compute_subnetwork.private.id }
output "connector_id" { value = google_vpc_access_connector.this.id }
