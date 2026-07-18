resource "aws_secretsmanager_secret" "db_url" {
  name       = "${var.name}/database-url"
  kms_key_id = var.kms_key_arn

  # A real rotation setup would attach a Lambda rotation function here
  # (rotation_rules + rotation_lambda_arn) that rotates the RDS master
  # password and rewrites this secret on a schedule. Not built for this
  # exercise - see DECISIONS.md "left out" section.

  tags = var.tags
}

resource "aws_secretsmanager_secret_version" "db_url" {
  secret_id = aws_secretsmanager_secret.db_url.id
  secret_string = jsonencode({
    DATABASE_URL = "postgres://${var.db_username}:${var.db_password}@${var.db_endpoint}:${var.db_port}/${var.db_name}?sslmode=require"
  })
}

# Consumed in-cluster by External Secrets Operator's ExternalSecret CR,
# which syncs this value into a native Kubernetes Secret that the
# bank-service pod mounts as env vars - see deploy/helm and DECISIONS.md.
