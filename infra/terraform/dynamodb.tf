# Vehicle credentials for NTRIP Basic auth against the relay (F-1/F-2 in
# rtk_relay_server_requirements.md). Items are managed out-of-band
# (console/CLI), not by Terraform, since they change per-vehicle over time.
resource "aws_dynamodb_table" "vehicle_credentials" {
  name         = "${var.project_name}-vehicle-credentials-${var.environment}"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "vehicle_id"

  attribute {
    name = "vehicle_id"
    type = "S"
  }

  point_in_time_recovery {
    enabled = true
  }
}
