resource "aws_ecs_cluster" "this" {
  name = "${var.project_name}-${var.environment}"
}

resource "aws_cloudwatch_log_group" "relay" {
  name              = "/ecs/${var.project_name}-${var.environment}"
  retention_in_days = 30
}

data "aws_iam_policy_document" "ecs_assume_role" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "execution" {
  name               = "${var.project_name}-execution-${var.environment}"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume_role.json
}

resource "aws_iam_role_policy_attachment" "execution_managed" {
  role       = aws_iam_role.execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_iam_role" "task" {
  name               = "${var.project_name}-task-${var.environment}"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume_role.json
}

data "aws_iam_policy_document" "task_permissions" {
  statement {
    sid       = "ReadVehicleCredentials"
    actions   = ["dynamodb:GetItem"]
    resources = [aws_dynamodb_table.vehicle_credentials.arn]
  }

  statement {
    sid       = "ReadProviderCredentials"
    actions   = ["secretsmanager:GetSecretValue"]
    resources = [local.provider_secrets_arn_pattern]
  }
}

resource "aws_iam_role_policy" "task_permissions" {
  name   = "${var.project_name}-task-${var.environment}"
  role   = aws_iam_role.task.id
  policy = data.aws_iam_policy_document.task_permissions.json
}

resource "aws_ecs_task_definition" "relay" {
  family                   = "${var.project_name}-${var.environment}"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = var.relay_cpu
  memory                   = var.relay_memory
  execution_role_arn       = aws_iam_role.execution.arn
  task_role_arn            = aws_iam_role.task.arn

  container_definitions = jsonencode([
    {
      name      = "relay"
      image     = "${aws_ecr_repository.relay.repository_url}:${var.relay_image_tag}"
      essential = true
      portMappings = [
        {
          containerPort = var.relay_container_port
          protocol      = "tcp"
        }
      ]
      environment = [
        { name = "LISTEN_ADDR", value = ":${var.relay_container_port}" },
        { name = "PROVIDER_SECRET_PREFIX", value = var.provider_secret_prefix },
        { name = "VEHICLE_TABLE_NAME", value = aws_dynamodb_table.vehicle_credentials.name },
      ]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = aws_cloudwatch_log_group.relay.name
          "awslogs-region"        = var.aws_region
          "awslogs-stream-prefix" = "relay"
        }
      }
    }
  ])
}

resource "aws_ecs_service" "relay" {
  name            = "${var.project_name}-${var.environment}"
  cluster         = aws_ecs_cluster.this.id
  task_definition = aws_ecs_task_definition.relay.arn
  desired_count   = var.relay_desired_count
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = data.aws_subnets.default.ids
    security_groups  = [aws_security_group.relay_task.id]
    assign_public_ip = true
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.relay.arn
    container_name   = "relay"
    container_port   = var.relay_container_port
  }

  depends_on = [aws_lb_listener.relay]
}
