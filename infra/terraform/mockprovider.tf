# mockprovider is a dummy RTK provider deployed purely to verify the
# relay end-to-end on AWS while no real provider is selected yet
# (rtk_provider_comparison.md). It is not reachable from the internet -
# only relay_task's security group may connect to it. Remove this file
# (and set deploy_mockprovider = false first, to clean up state) once a
# real provider is in use.

resource "aws_security_group" "mockprovider_task" {
  count       = var.deploy_mockprovider ? 1 : 0
  name        = "${var.project_name}-mockprovider-${var.environment}"
  description = "Allows inbound NTRIP traffic from the relay task only (POC verification, not internet-facing)"
  vpc_id      = data.aws_vpc.default.id

  ingress {
    description     = "NTRIP from relay task"
    from_port       = var.mockprovider_container_port
    to_port         = var.mockprovider_container_port
    protocol        = "tcp"
    security_groups = [aws_security_group.relay_task.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_ecs_task_definition" "mockprovider" {
  count                    = var.deploy_mockprovider ? 1 : 0
  family                   = "${var.project_name}-mockprovider-${var.environment}"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = 256
  memory                   = 512
  execution_role_arn       = aws_iam_role.execution.arn
  task_role_arn            = aws_iam_role.task.arn

  container_definitions = jsonencode([
    {
      name      = "mockprovider"
      image     = "${aws_ecr_repository.relay.repository_url}:${var.mockprovider_image_tag}"
      essential = true
      portMappings = [
        {
          containerPort = var.mockprovider_container_port
          protocol      = "tcp"
        }
      ]
      environment = [
        { name = "LISTEN_ADDR", value = ":${var.mockprovider_container_port}" },
        { name = "MOCK_PROVIDER_MOUNTPOINT", value = var.mockprovider_mountpoint },
        { name = "MOCK_PROVIDER_USERNAME", value = var.mockprovider_username },
        { name = "MOCK_PROVIDER_PASSWORD", value = var.mockprovider_password },
        { name = "MOCK_PROVIDER_REQUIRE_HEADER_KEY", value = var.mockprovider_required_header_key },
        { name = "MOCK_PROVIDER_REQUIRE_HEADER_VALUE", value = var.mockprovider_required_header_value },
      ]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = aws_cloudwatch_log_group.relay.name
          "awslogs-region"        = var.aws_region
          "awslogs-stream-prefix" = "mockprovider"
        }
      }
    }
  ])
}

resource "aws_ecs_service" "mockprovider" {
  count           = var.deploy_mockprovider ? 1 : 0
  name            = "${var.project_name}-mockprovider-${var.environment}"
  cluster         = aws_ecs_cluster.this.id
  task_definition = aws_ecs_task_definition.mockprovider[0].arn
  desired_count   = 1
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = data.aws_subnets.default.ids
    security_groups  = [aws_security_group.mockprovider_task[0].id]
    assign_public_ip = true
  }
}
