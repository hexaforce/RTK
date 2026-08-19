# POC uses the account's default VPC/subnets to avoid NAT Gateway cost;
# Fargate tasks get public IPs directly instead of routing through a NAT.
data "aws_vpc" "default" {
  default = true
}

data "aws_subnets" "default" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.default.id]
  }
}

resource "aws_security_group" "nlb" {
  name        = "${var.project_name}-nlb-${var.environment}"
  description = "Allows inbound NTRIP traffic from vehicles to the relay NLB"
  vpc_id      = data.aws_vpc.default.id

  ingress {
    description = "NTRIP from vehicles"
    from_port   = var.relay_container_port
    to_port     = var.relay_container_port
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_security_group" "relay_task" {
  name        = "${var.project_name}-task-${var.environment}"
  description = "Allows inbound NTRIP traffic from the NLB to the relay task"
  vpc_id      = data.aws_vpc.default.id

  ingress {
    description     = "NTRIP from NLB"
    from_port       = var.relay_container_port
    to_port         = var.relay_container_port
    protocol        = "tcp"
    security_groups = [aws_security_group.nlb.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}
