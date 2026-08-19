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

variable "deploy_mockprovider" {
  description = "Whether to deploy the mockprovider dummy RTK provider for end-to-end verification while no real provider is selected. Set to false (or remove the resources) once a real provider is in use."
  type        = bool
  default     = true
}

variable "mockprovider_image_tag" {
  description = "Tag of the mockprovider image in ECR to deploy."
  type        = string
  default     = "mockprovider"
}

variable "mockprovider_container_port" {
  description = "TCP port the mockprovider listens on."
  type        = number
  default     = 2201
}

variable "mockprovider_mountpoint" {
  description = "NTRIP mountpoint the mockprovider expects the relay to request."
  type        = string
  default     = "TESTMOUNT"
}

variable "mockprovider_username" {
  description = "Basic auth username the mockprovider expects from the relay."
  type        = string
  default     = "relay"
}

variable "mockprovider_password" {
  description = "Basic auth password the mockprovider expects from the relay. Dummy value for POC verification only."
  type        = string
  default     = "relay-secret"
  sensitive   = true
}

variable "provider_secret_prefix" {
  description = "Secrets Manager name prefix under which one secret per RTK provider lives (name = prefix + provider_id). The relay task role is granted read access to this whole prefix, so adding a provider is just creating a new secret here - no Terraform change needed."
  type        = string
  default     = "rtk-relay/providers/"
}

variable "relay_subdomain" {
  description = "Subdomain (under fpv.jp, managed at Value-Domain) that will CNAME to the relay's NLB. Not created by Terraform - see outputs.nlb_dns_name for the manual DNS step."
  type        = string
  default     = "rtk.fpv.jp"
}
