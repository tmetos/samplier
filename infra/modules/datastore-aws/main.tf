# RDS for PostgreSQL with PostGIS, Multi-AZ, in private subnets.

variable "name" { type = string }
variable "vpc_id" { type = string }
variable "private_subnet_ids" { type = list(string) }
variable "app_security_group_id" { type = string }
variable "instance_class" { type = string }
variable "allocated_storage" { type = number }
variable "max_allocated_storage" { type = number }
variable "multi_az" { type = bool }
variable "backup_retention_days" { type = number }
variable "tags" { type = map(string) }

resource "random_password" "db" {
  length  = 32
  special = false
}

resource "random_password" "keycloak_admin" {
  length  = 32
  special = false
}

resource "aws_db_subnet_group" "this" {
  name       = "${var.name}-db"
  subnet_ids = var.private_subnet_ids
  tags       = merge(var.tags, { Name = "${var.name}-db-subnets" })
}

resource "aws_security_group" "db" {
  name        = "${var.name}-db"
  description = "PostgreSQL access from application tasks"
  vpc_id      = var.vpc_id

  ingress {
    description     = "PostgreSQL from app tasks"
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [var.app_security_group_id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = merge(var.tags, { Name = "${var.name}-db" })
}

# PostGIS is created by the schema's CREATE EXTENSION; RDS PostgreSQL 16 ships it.
resource "aws_db_parameter_group" "this" {
  name   = "${var.name}-pg16"
  family = "postgres16"

  parameter {
    name  = "rds.force_ssl"
    value = "1"
  }

  parameter {
    name  = "log_min_duration_statement"
    value = "1000"
  }

  tags = var.tags
}

resource "aws_db_instance" "this" {
  identifier = "${var.name}-db"

  engine         = "postgres"
  engine_version = "16"
  instance_class = var.instance_class

  allocated_storage     = var.allocated_storage
  max_allocated_storage = var.max_allocated_storage
  storage_type          = "gp3"
  storage_encrypted     = true

  db_name  = "samplier"
  username = "samplier"
  password = random_password.db.result

  multi_az               = var.multi_az
  db_subnet_group_name   = aws_db_subnet_group.this.name
  vpc_security_group_ids = [aws_security_group.db.id]
  parameter_group_name   = aws_db_parameter_group.this.name

  backup_retention_period   = var.backup_retention_days
  backup_window             = "07:00-08:00"
  maintenance_window        = "sun:08:00-sun:09:00"
  deletion_protection       = true
  skip_final_snapshot       = false
  final_snapshot_identifier = "${var.name}-db-final"

  performance_insights_enabled = true
  auto_minor_version_upgrade   = true

  tags = merge(var.tags, { Name = "${var.name}-db" })
}

resource "aws_secretsmanager_secret" "db" {
  name        = "${var.name}/db"
  description = "Samplier database credentials"
  tags        = var.tags
}

resource "aws_secretsmanager_secret_version" "db" {
  secret_id = aws_secretsmanager_secret.db.id
  secret_string = jsonencode({
    username = aws_db_instance.this.username
    password = random_password.db.result
    host     = aws_db_instance.this.address
    port     = aws_db_instance.this.port
    dbname   = aws_db_instance.this.db_name
    url      = "postgres://${aws_db_instance.this.username}:${random_password.db.result}@${aws_db_instance.this.address}:${aws_db_instance.this.port}/${aws_db_instance.this.db_name}?sslmode=require"
  })
}

resource "aws_secretsmanager_secret" "keycloak_admin" {
  name        = "${var.name}/keycloak-admin"
  description = "Keycloak bootstrap admin credentials"
  tags        = var.tags
}

resource "aws_secretsmanager_secret_version" "keycloak_admin" {
  secret_id = aws_secretsmanager_secret.keycloak_admin.id
  secret_string = jsonencode({
    username = "admin"
    password = random_password.keycloak_admin.result
  })
}

output "endpoint" { value = aws_db_instance.this.address }
output "port" { value = aws_db_instance.this.port }
output "database_name" { value = aws_db_instance.this.db_name }
output "instance_id" { value = aws_db_instance.this.identifier }
output "secret_arn" { value = aws_secretsmanager_secret.db.arn }
output "keycloak_admin_secret_arn" { value = aws_secretsmanager_secret.keycloak_admin.arn }
