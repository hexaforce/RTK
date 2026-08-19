output "ecr_repository_url" {
  description = "Push relay images here before applying/updating the ECS service."
  value       = aws_ecr_repository.relay.repository_url
}

output "nlb_dns_name" {
  description = "Create a CNAME for var.relay_subdomain at Value-Domain pointing to this."
  value       = aws_lb.relay.dns_name
}

output "ecs_cluster_name" {
  value = aws_ecs_cluster.this.name
}

output "ecs_service_name" {
  value = aws_ecs_service.relay.name
}

output "vehicle_credentials_table_name" {
  value = aws_dynamodb_table.vehicle_credentials.name
}

output "provider_credentials_secret_arn" {
  value = aws_secretsmanager_secret.provider_credentials.arn
}
