# ECS Fargate services (API + Keycloak) behind an ALB, with autoscaling,
# CloudWatch logging, and Container Insights.

variable "name" { type = string }
variable "vpc_id" { type = string }
variable "public_subnet_ids" { type = list(string) }
variable "private_subnet_ids" { type = list(string) }
variable "alb_security_group_id" { type = string }
variable "app_security_group_id" { type = string }

variable "api_image" { type = string }
variable "keycloak_image" { type = string }
variable "api_cpu" { type = number }
variable "api_memory" { type = number }

variable "desired_count" { type = number }
variable "min_count" { type = number }
variable "max_count" { type = number }

variable "db_endpoint" { type = string }
variable "db_name" { type = string }
variable "db_secret_arn" { type = string }
variable "keycloak_admin_secret_arn" { type = string }

variable "certificate_arn" { type = string }
variable "log_retention_days" { type = number }
variable "domain_name" { type = string }
variable "tags" { type = map(string) }

# --- logging ---------------------------------------------------------------

resource "aws_cloudwatch_log_group" "api" {
  name              = "/ecs/${var.name}/api"
  retention_in_days = var.log_retention_days
  tags              = var.tags
}

resource "aws_cloudwatch_log_group" "keycloak" {
  name              = "/ecs/${var.name}/keycloak"
  retention_in_days = var.log_retention_days
  tags              = var.tags
}

# --- cluster ---------------------------------------------------------------

resource "aws_ecs_cluster" "this" {
  name = var.name

  setting {
    name  = "containerInsights"
    value = "enabled"
  }

  tags = var.tags
}

resource "aws_ecs_cluster_capacity_providers" "this" {
  cluster_name       = aws_ecs_cluster.this.name
  capacity_providers = ["FARGATE", "FARGATE_SPOT"]

  default_capacity_provider_strategy {
    capacity_provider = "FARGATE"
    weight            = 1
  }
}

# --- load balancer ---------------------------------------------------------

resource "aws_lb" "this" {
  name               = "${var.name}-alb"
  load_balancer_type = "application"
  security_groups    = [var.alb_security_group_id]
  subnets            = var.public_subnet_ids

  tags = var.tags
}

resource "aws_lb_target_group" "api" {
  name        = "${var.name}-api"
  port        = 8080
  protocol    = "HTTP"
  vpc_id      = var.vpc_id
  target_type = "ip"

  health_check {
    path                = "/healthz"
    matcher             = "200"
    interval            = 30
    timeout             = 5
    healthy_threshold   = 2
    unhealthy_threshold = 3
  }

  tags = var.tags
}

resource "aws_lb_target_group" "keycloak" {
  name        = "${var.name}-keycloak"
  port        = 8080
  protocol    = "HTTP"
  vpc_id      = var.vpc_id
  target_type = "ip"

  health_check {
    path                = "/health/ready"
    matcher             = "200"
    interval            = 30
    timeout             = 5
    healthy_threshold   = 2
    unhealthy_threshold = 5
  }

  tags = var.tags
}

resource "aws_lb_listener" "http" {
  load_balancer_arn = aws_lb.this.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type = "redirect"
    redirect {
      port        = "443"
      protocol    = "HTTPS"
      status_code = "HTTP_301"
    }
  }
}

resource "aws_lb_listener" "https" {
  load_balancer_arn = aws_lb.this.arn
  port              = 443
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"
  # The certificate is created at the root module; pass it via a data lookup so
  # the module stays reusable.
  certificate_arn = var.certificate_arn

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.api.arn
  }
}

resource "aws_lb_listener_rule" "ogc" {
  listener_arn = aws_lb_listener.https.arn
  priority     = 10

  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.api.arn
  }

  condition {
    path_pattern {
      values = ["/ogc/*", "/ogc"]
    }
  }
}

resource "aws_lb_listener_rule" "auth" {
  listener_arn = aws_lb_listener.https.arn
  priority     = 20

  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.keycloak.arn
  }

  condition {
    path_pattern {
      values = ["/auth/*", "/auth"]
    }
  }
}

# --- IAM -------------------------------------------------------------------

data "aws_iam_policy_document" "ecs_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "execution" {
  name               = "${var.name}-ecs-execution"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume.json
  tags               = var.tags
}

resource "aws_iam_role_policy_attachment" "execution_managed" {
  role       = aws_iam_role.execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_iam_role_policy" "secrets" {
  name = "${var.name}-secrets"
  role = aws_iam_role.execution.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Action = ["secretsmanager:GetSecretValue"]
      Resource = [
        var.db_secret_arn,
        var.keycloak_admin_secret_arn,
      ]
    }]
  })
}

# --- task definitions ------------------------------------------------------

resource "aws_ecs_task_definition" "api" {
  family                   = "${var.name}-api"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = var.api_cpu
  memory                   = var.api_memory
  execution_role_arn       = aws_iam_role.execution.arn
  task_role_arn            = aws_iam_role.execution.arn

  container_definitions = jsonencode([{
    name      = "api"
    image     = var.api_image
    essential = true

    portMappings = [{ containerPort = 8080, protocol = "tcp" }]

    environment = [
      { name = "PORT", value = "8080" },
      { name = "AUTH_ENABLED", value = "true" },
      { name = "OIDC_ISSUER", value = "https://${var.domain_name}/auth/realms/samplier" },
      { name = "OIDC_CLIENT_ID", value = "samplier-ui" },
      { name = "LOG_LEVEL", value = "info" },
    ]

    secrets = [
      { name = "DATABASE_URL", valueFrom = "${var.db_secret_arn}:url::" },
    ]

    logConfiguration = {
      logDriver = "awslogs"
      options = {
        "awslogs-group"         = aws_cloudwatch_log_group.api.name
        "awslogs-region"        = data.aws_region.current.name
        "awslogs-stream-prefix" = "api"
      }
    }

    healthCheck = {
      command     = ["CMD", "/app/samplier", "version"]
      interval    = 30
      timeout     = 5
      retries     = 3
      startPeriod = 15
    }
  }])

  tags = var.tags
}

resource "aws_ecs_task_definition" "keycloak" {
  family                   = "${var.name}-keycloak"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = 1024
  memory                   = 2048
  execution_role_arn       = aws_iam_role.execution.arn
  task_role_arn            = aws_iam_role.execution.arn

  container_definitions = jsonencode([{
    name      = "keycloak"
    image     = var.keycloak_image
    essential = true

    command = ["start", "--optimized", "--http-enabled=true", "--proxy-headers=xforwarded"]

    portMappings = [{ containerPort = 8080, protocol = "tcp" }]

    environment = [
      { name = "KC_DB", value = "postgres" },
      { name = "KC_DB_URL", value = "jdbc:postgresql://${var.db_endpoint}:5432/${var.db_name}" },
      { name = "KC_DB_USERNAME", value = "samplier" },
      { name = "KC_HOSTNAME", value = "https://${var.domain_name}/auth" },
      { name = "KC_HTTP_RELATIVE_PATH", value = "/auth" },
      { name = "KC_HEALTH_ENABLED", value = "true" },
      { name = "KC_METRICS_ENABLED", value = "true" },
      { name = "KC_PROXY_HEADERS", value = "xforwarded" },
    ]

    secrets = [
      { name = "KC_DB_PASSWORD", valueFrom = "${var.db_secret_arn}:password::" },
      { name = "KEYCLOAK_ADMIN", valueFrom = "${var.keycloak_admin_secret_arn}:username::" },
      { name = "KEYCLOAK_ADMIN_PASSWORD", valueFrom = "${var.keycloak_admin_secret_arn}:password::" },
    ]

    logConfiguration = {
      logDriver = "awslogs"
      options = {
        "awslogs-group"         = aws_cloudwatch_log_group.keycloak.name
        "awslogs-region"        = data.aws_region.current.name
        "awslogs-stream-prefix" = "keycloak"
      }
    }
  }])

  tags = var.tags
}

data "aws_region" "current" {}

# --- services --------------------------------------------------------------

resource "aws_ecs_service" "api" {
  name            = "${var.name}-api"
  cluster         = aws_ecs_cluster.this.id
  task_definition = aws_ecs_task_definition.api.arn
  desired_count   = var.desired_count
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = var.private_subnet_ids
    security_groups  = [var.app_security_group_id]
    assign_public_ip = false
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.api.arn
    container_name   = "api"
    container_port   = 8080
  }

  health_check_grace_period_seconds = 30
  enable_execute_command            = true

  depends_on = [aws_lb_listener.https]

  tags = var.tags
}

resource "aws_ecs_service" "keycloak" {
  name            = "${var.name}-keycloak"
  cluster         = aws_ecs_cluster.this.id
  task_definition = aws_ecs_task_definition.keycloak.arn
  desired_count   = 1
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = var.private_subnet_ids
    security_groups  = [var.app_security_group_id]
    assign_public_ip = false
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.keycloak.arn
    container_name   = "keycloak"
    container_port   = 8080
  }

  health_check_grace_period_seconds = 120
  enable_execute_command            = true

  depends_on = [aws_lb_listener.https]

  tags = var.tags
}

# --- autoscaling -----------------------------------------------------------

resource "aws_appautoscaling_target" "api" {
  service_namespace  = "ecs"
  resource_id        = "service/${aws_ecs_cluster.this.name}/${aws_ecs_service.api.name}"
  scalable_dimension = "ecs:service:DesiredCount"
  min_capacity       = var.min_count
  max_capacity       = var.max_count
}

resource "aws_appautoscaling_policy" "api_cpu" {
  name               = "${var.name}-api-cpu"
  policy_type        = "TargetTrackingScaling"
  service_namespace  = aws_appautoscaling_target.api.service_namespace
  resource_id        = aws_appautoscaling_target.api.resource_id
  scalable_dimension = aws_appautoscaling_target.api.scalable_dimension

  target_tracking_scaling_policy_configuration {
    predefined_metric_specification {
      predefined_metric_type = "ECSServiceAverageCPUUtilization"
    }
    target_value       = 65
    scale_in_cooldown  = 300
    scale_out_cooldown = 60
  }
}

resource "aws_appautoscaling_policy" "api_requests" {
  name               = "${var.name}-api-requests"
  policy_type        = "TargetTrackingScaling"
  service_namespace  = aws_appautoscaling_target.api.service_namespace
  resource_id        = aws_appautoscaling_target.api.resource_id
  scalable_dimension = aws_appautoscaling_target.api.scalable_dimension

  target_tracking_scaling_policy_configuration {
    predefined_metric_specification {
      predefined_metric_type = "ALBRequestCountPerTarget"
      resource_label         = "${aws_lb.this.arn_suffix}/${aws_lb_target_group.api.arn_suffix}"
    }
    target_value       = 500
    scale_in_cooldown  = 300
    scale_out_cooldown = 60
  }
}

output "alb_dns_name" { value = aws_lb.this.dns_name }
output "alb_zone_id" { value = aws_lb.this.zone_id }
output "alb_arn_suffix" { value = aws_lb.this.arn_suffix }
output "api_target_group_arn_suffix" { value = aws_lb_target_group.api.arn_suffix }
output "cluster_name" { value = aws_ecs_cluster.this.name }
output "api_service_name" { value = aws_ecs_service.api.name }
