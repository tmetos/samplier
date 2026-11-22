terraform {
  required_version = ">= 1.6"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 6.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
  }

  # Remote state: uncomment after creating the bucket.
  # backend "gcs" {
  #   bucket = "samplier-tfstate"
  #   prefix = "gcp/prod"
  # }
}

provider "google" {
  project = var.project_id
  region  = var.region
}

provider "random" {}

locals {
  tags = {
    project     = var.project
    environment = var.environment
    managed_by  = "terraform"
  }
  name = "${var.project}-${var.environment}"
}

module "network" {
  source = "../modules/network-gcp"

  project_id = var.project_id
  region     = var.region
  name       = local.name
  tags       = local.tags
}

module "datastore" {
  source = "../modules/datastore-gcp"

  project_id            = var.project_id
  region                = var.region
  name                  = local.name
  network_id            = module.network.network_id
  tier                  = var.sql_tier
  availability_type     = var.sql_availability_type
  disk_size_gb          = var.sql_disk_size_gb
  backup_retention_days = var.sql_backup_retention_days
  tags                  = local.tags
}

module "app" {
  source = "../modules/app-gcp"

  project_id   = var.project_id
  region       = var.region
  name         = local.name
  network_id   = module.network.network_id
  connector_id = module.network.connector_id

  api_image      = var.api_image
  keycloak_image = var.keycloak_image

  db_private_ip            = module.datastore.private_ip
  db_name                  = module.datastore.database_name
  db_secret_id             = module.datastore.db_secret_id
  keycloak_admin_secret_id = module.datastore.keycloak_admin_secret_id

  domain_name        = var.domain_name
  min_instances      = var.min_instances
  max_instances      = var.max_instances
  log_retention_days = var.log_retention_days

  tags = local.tags

  depends_on = [module.network]
}

# --- DNS (Terraform-managed, per spec) -------------------------------------

data "google_dns_managed_zone" "this" {
  count   = var.dns_zone_name == "" ? 0 : 1
  name    = var.dns_zone_name
  project = var.project_id
}

resource "google_dns_record_set" "app" {
  count = var.dns_zone_name == "" ? 0 : 1

  name         = "${var.domain_name}."
  managed_zone = data.google_dns_managed_zone.this[0].name
  project      = var.project_id
  type         = "A"
  ttl          = 300
  rrdatas      = [module.app.load_balancer_ip]
}
