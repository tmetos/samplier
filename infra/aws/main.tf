terraform {
  required_version = ">= 1.6"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.60"
    }
  }

  # Remote state: uncomment and adjust after creating the bucket/table.
  # backend "s3" {
  #   bucket         = "samplier-tfstate"
  #   key            = "aws/prod/terraform.tfstate"
  #   region         = "us-west-2"
  #   dynamodb_table = "samplier-tflock"
  #   encrypt        = true
  # }
}

provider "aws" {
  region = var.region

  default_tags {
    tags = local.tags
  }
}

data "aws_availability_zones" "available" {
  state = "available"
}

data "aws_caller_identity" "current" {}

locals {
  tags = {
    Project     = var.project
    Environment = var.environment
    ManagedBy   = "terraform"
  }

  name = "${var.project}-${var.environment}"

  azs = slice(data.aws_availability_zones.available.names, 0, 2)

  # /16 network split into /24 subnets.
  public_subnets  = [cidrsubnet(var.vpc_cidr, 8, 0), cidrsubnet(var.vpc_cidr, 8, 1)]
  private_subnets = [cidrsubnet(var.vpc_cidr, 8, 10), cidrsubnet(var.vpc_cidr, 8, 11)]
}

module "network" {
  source = "../modules/network-aws"

  name            = local.name
  vpc_cidr        = var.vpc_cidr
  azs             = local.azs
  public_subnets  = local.public_subnets
  private_subnets = local.private_subnets
  tags            = local.tags
}

module "datastore" {
  source = "../modules/datastore-aws"

  name                  = local.name
  vpc_id                = module.network.vpc_id
  private_subnet_ids    = module.network.private_subnet_ids
  app_security_group_id = module.network.app_security_group_id
  instance_class        = var.db_instance_class
  allocated_storage     = var.db_allocated_storage
  max_allocated_storage = var.db_max_allocated_storage
  multi_az              = var.db_multi_az
  backup_retention_days = var.db_backup_retention_days
  tags                  = local.tags
}

resource "aws_acm_certificate" "this" {
  domain_name       = var.domain_name
  validation_method = "DNS"

  lifecycle {
    create_before_destroy = true
  }

  tags = local.tags
}

resource "aws_route53_record" "validation" {
  for_each = {
    for dvo in aws_acm_certificate.this.domain_validation_options : dvo.domain_name => {
      name   = dvo.resource_record_name
      record = dvo.resource_record_value
      type   = dvo.resource_record_type
    }
  }

  zone_id = var.route53_zone_id
  name    = each.value.name
  type    = each.value.type
  records = [each.value.record]
  ttl     = 60
}

resource "aws_acm_certificate_validation" "this" {
  certificate_arn         = aws_acm_certificate.this.arn
  validation_record_fqdns = [for r in aws_route53_record.validation : r.fqdn]
}

module "app" {
  source = "../modules/app-aws"

  name                  = local.name
  vpc_id                = module.network.vpc_id
  public_subnet_ids     = module.network.public_subnet_ids
  private_subnet_ids    = module.network.private_subnet_ids
  alb_security_group_id = module.network.alb_security_group_id
  app_security_group_id = module.network.app_security_group_id

  api_image      = var.api_image
  keycloak_image = var.keycloak_image
  api_cpu        = var.api_cpu
  api_memory     = var.api_memory

  desired_count = var.api_desired_count
  min_count     = var.api_min_count
  max_count     = var.api_max_count

  db_endpoint               = module.datastore.endpoint
  db_name                   = module.datastore.database_name
  db_secret_arn             = module.datastore.secret_arn
  keycloak_admin_secret_arn = module.datastore.keycloak_admin_secret_arn

  certificate_arn    = aws_acm_certificate_validation.this.certificate_arn
  log_retention_days = var.log_retention_days
  domain_name        = var.domain_name

  tags = local.tags
}

resource "aws_route53_record" "app" {
  zone_id = var.route53_zone_id
  name    = var.domain_name
  type    = "A"

  alias {
    name                   = module.app.alb_dns_name
    zone_id                = module.app.alb_zone_id
    evaluate_target_health = true
  }
}

resource "aws_cloudwatch_metric_alarm" "api_5xx" {
  alarm_name          = "${local.name}-api-5xx"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 2
  metric_name         = "HTTPCode_Target_5XX_Count"
  namespace           = "AWS/ApplicationELB"
  period              = 300
  statistic           = "Sum"
  threshold           = 10
  alarm_description   = "Samplier API is returning 5xx responses"
  treat_missing_data  = "notBreaching"

  dimensions = {
    LoadBalancer = module.app.alb_arn_suffix
  }
}

resource "aws_cloudwatch_metric_alarm" "api_latency" {
  alarm_name          = "${local.name}-api-p95-latency"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 3
  metric_name         = "TargetResponseTime"
  namespace           = "AWS/ApplicationELB"
  period              = 300
  extended_statistic  = "p95"
  threshold           = 2
  alarm_description   = "Samplier API p95 latency above 2s"
  treat_missing_data  = "notBreaching"

  dimensions = {
    LoadBalancer = module.app.alb_arn_suffix
  }
}

resource "aws_cloudwatch_metric_alarm" "db_cpu" {
  alarm_name          = "${local.name}-db-cpu"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 3
  metric_name         = "CPUUtilization"
  namespace           = "AWS/RDS"
  period              = 300
  statistic           = "Average"
  threshold           = 80
  alarm_description   = "RDS CPU above 80%"
  treat_missing_data  = "notBreaching"

  dimensions = {
    DBInstanceIdentifier = module.datastore.instance_id
  }
}

resource "aws_cloudwatch_metric_alarm" "db_connections" {
  alarm_name          = "${local.name}-db-connections"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 2
  metric_name         = "DatabaseConnections"
  namespace           = "AWS/RDS"
  period              = 300
  statistic           = "Average"
  threshold           = var.db_max_connections_alarm
  alarm_description   = "RDS connection count nearing the instance limit"
  treat_missing_data  = "notBreaching"

  dimensions = {
    DBInstanceIdentifier = module.datastore.instance_id
  }
}
