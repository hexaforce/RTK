# RTK provider credentials (rtk_relay_protocol_design.md, "プロバイダー側認証").
# Terraform only creates the secret container - the value is populated
# manually once a provider is selected (rtk_provider_comparison.md), so
# provider credentials never pass through Terraform state or this repo.
# Populate it with:
#   aws secretsmanager put-secret-value --secret-id <arn> \
#     --secret-string '{"host":"...","port":"2101","mountpoint":"...","username":"...","password":"..."}' \
#     --profile fpv-japan --region ap-northeast-1
resource "aws_secretsmanager_secret" "provider_credentials" {
  name        = "${var.project_name}/provider-credentials-${var.environment}"
  description = "RTK provider NTRIP connection details (host/port/mountpoint/username/password)"
}
