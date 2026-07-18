output "vpc_id" {
  value = module.network.vpc_id
}

output "eks_cluster_name" {
  value = module.eks.cluster_name
}

output "eks_cluster_endpoint" {
  value = module.eks.cluster_endpoint
}

output "ecr_repository_url" {
  value = module.ecr.repository_url
}

output "rds_endpoint" {
  value = module.rds.endpoint
}

output "secret_arn" {
  value = module.secrets.secret_arn
}

output "irsa_role_arn" {
  value = module.iam.irsa_role_arn
}

# The three GitHub Actions OIDC role ARNs, one per workflow-scoped
# permission tier - copy these into the matching GitHub repo/environment
# variables (CI_DEPLOY_ROLE_ARN, TF_PLAN_ROLE_ARN, TF_APPLY_ROLE_ARN) once
# this stack is actually applied. See modules/iam and DECISIONS.md.
output "ci_deploy_role_arn" {
  value = module.iam.ci_deploy_role_arn
}

output "terraform_plan_role_arn" {
  value = module.iam.terraform_plan_role_arn
}

output "terraform_apply_role_arn" {
  value = module.iam.terraform_apply_role_arn
}

output "kubectl_config_command" {
  value = "aws eks update-kubeconfig --name ${module.eks.cluster_name} --region ${var.aws_region}"
}
