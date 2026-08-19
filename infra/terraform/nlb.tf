# NLB (TCP passthrough) instead of ALB, since NTRIP is a long-lived
# streaming connection, not request/response HTTP semantics
# (see rtk_relay_server_requirements.md, "AWS構成案").
resource "aws_lb" "relay" {
  name               = "${var.project_name}-${var.environment}"
  internal           = false
  load_balancer_type = "network"
  security_groups    = [aws_security_group.nlb.id]
  subnets            = data.aws_subnets.default.ids
}

resource "aws_lb_target_group" "relay" {
  name        = "${var.project_name}-${var.environment}"
  port        = var.relay_container_port
  protocol    = "TCP"
  target_type = "ip"
  vpc_id      = data.aws_vpc.default.id

  health_check {
    protocol = "TCP"
    port     = "traffic-port"
  }
}

resource "aws_lb_listener" "relay" {
  load_balancer_arn = aws_lb.relay.arn
  port              = var.relay_container_port
  protocol          = "TCP"

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.relay.arn
  }
}
