# RTK provider credentials, one Secrets Manager secret per provider
# (rtk_relay_protocol_design.md, "プロバイダー側認証"). Terraform does not
# create individual provider secrets - it only grants the relay task
# read access to anything under var.provider_secret_prefix, so adding,
# rotating or removing a provider is a plain `aws secretsmanager`
# call, not a Terraform change (this is what makes per-vehicle,
# multi-provider assignment - see the vehicle_id item's provider_id
# attribute in dynamodb.tf - operable without redeploying).
#
# Create a provider's secret with:
#   aws secretsmanager create-secret --name "${var.provider_secret_prefix}<provider_id>" \
#     --secret-string '{"host":"...","port":"2101","mountpoint":"...","username":"...","password":"..."}' \
#     --profile rtk-micros-dev --region ap-northeast-1

data "aws_caller_identity" "current" {}

locals {
  provider_secrets_arn_pattern = "arn:aws:secretsmanager:${var.aws_region}:${data.aws_caller_identity.current.account_id}:secret:${var.provider_secret_prefix}*"
}
