output "secret_arn" {
  value = aws_secretsmanager_secret.db_url.arn
}

output "secret_name" {
  value = aws_secretsmanager_secret.db_url.name
}
