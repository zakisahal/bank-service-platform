output "irsa_role_arn" {
  value = aws_iam_role.irsa.arn
}

output "ci_deploy_role_arn" {
  value = var.enable_github_oidc ? aws_iam_role.ci_deploy[0].arn : null
}

output "terraform_plan_role_arn" {
  value = var.enable_github_oidc ? aws_iam_role.terraform_plan[0].arn : null
}

output "terraform_apply_role_arn" {
  value = var.enable_github_oidc ? aws_iam_role.terraform_apply[0].arn : null
}
