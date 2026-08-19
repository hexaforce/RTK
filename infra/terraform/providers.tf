provider "aws" {
  region  = var.aws_region
  profile = var.aws_profile

  default_tags {
    tags = {
      Project     = "rtk-relay"
      Environment = var.environment
      ManagedBy   = "terraform"
    }
  }
}
