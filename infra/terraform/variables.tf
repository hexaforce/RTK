variable "aws_region" {
  description = "AWS region for all resources."
  type        = string
  default     = "ap-northeast-1"
}

variable "aws_profile" {
  description = "AWS CLI profile (see ~/.aws/credentials) used by Terraform."
  type        = string
  default     = "fpv-japan"
}

variable "environment" {
  description = "Deployment environment name, used in resource naming/tags."
  type        = string
  default     = "poc"
}

variable "project_name" {
  description = "Short project name used as a prefix for resource names."
  type        = string
  default     = "rtk-relay"
}

variable "relay_image_tag" {
  description = "Tag of the rtk-relay image in ECR to deploy. Push an image with this tag before applying, or the ECS service will fail to start."
  type        = string
  default     = "latest"
}

variable "relay_container_port" {
  description = "TCP port the relay listens on for NTRIP connections from vehicles."
  type        = number
  default     = 2101
}

variable "relay_desired_count" {
  description = "Number of Fargate tasks to run. Kept at 1 for the POC."
  type        = number
  default     = 1
}

variable "relay_cpu" {
  description = "Fargate task CPU units."
  type        = number
  default     = 256
}

variable "relay_memory" {
  description = "Fargate task memory (MiB)."
  type        = number
  default     = 512
}

variable "data_bucket_name" {
  description = "Existing S3 bucket for collected measurement data (created outside Terraform)."
  type        = string
  default     = "fpv-japan"
}

variable "relay_subdomain" {
  description = "Subdomain (under fpv.jp, managed at Value-Domain) that will CNAME to the relay's NLB. Not created by Terraform - see outputs.nlb_dns_name for the manual DNS step."
  type        = string
  default     = "rtk-relay.fpv.jp"
}
