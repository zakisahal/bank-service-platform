locals {
  name = "${var.project}-${var.environment}"

  common_tags = {
    Project     = var.project
    Environment = var.environment
    ManagedBy   = "terraform"
  }
}

resource "aws_kms_key" "main" {
  description             = "${local.name} - RDS, Secrets Manager, EKS secrets, CloudWatch Logs"
  enable_key_rotation     = true
  deletion_window_in_days = 30
  tags                    = local.common_tags
}

resource "aws_kms_alias" "main" {
  name          = "alias/${local.name}"
  target_key_id = aws_kms_key.main.key_id
}

# RDS master password. In a real account this would be generated once and
# then left alone (Secrets Manager rotation takes over from here) rather
# than being re-derived by Terraform on every plan.
resource "random_password" "db_master" {
  length           = 24
  special          = true
  override_special = "!#$%&*()-_=+"
}

module "network" {
  source = "../../modules/network"

  name                 = local.name
  vpc_cidr             = var.vpc_cidr
  availability_zones   = var.availability_zones
  public_subnet_cidrs  = var.public_subnet_cidrs
  private_subnet_cidrs = var.private_subnet_cidrs
  tags                 = local.common_tags
}

module "ecr" {
  source = "../../modules/ecr"

  name        = local.name
  kms_key_arn = aws_kms_key.main.arn
  tags        = local.common_tags
}

module "eks" {
  source = "../../modules/eks"

  name                   = local.name
  vpc_id                 = module.network.vpc_id
  private_subnet_ids     = module.network.private_subnet_ids
  kubernetes_version     = var.kubernetes_version
  endpoint_public_access = var.eks_endpoint_public_access
  kms_key_arn            = aws_kms_key.main.arn
  tags                   = local.common_tags
}

module "rds" {
  source = "../../modules/rds"

  name                       = local.name
  vpc_id                     = module.network.vpc_id
  private_subnet_ids         = module.network.private_subnet_ids
  allowed_security_group_ids = [module.eks.cluster_security_group_id]
  master_password            = random_password.db_master.result
  kms_key_arn                = aws_kms_key.main.arn
  multi_az                   = var.rds_multi_az
  deletion_protection        = var.rds_deletion_protection
  tags                       = local.common_tags
}

module "secrets" {
  source = "../../modules/secrets"

  name        = local.name
  kms_key_arn = aws_kms_key.main.arn
  db_username = "bank_app"
  db_password = random_password.db_master.result
  db_endpoint = module.rds.endpoint
  db_port     = module.rds.port
  db_name     = module.rds.database_name
  tags        = local.common_tags
}

module "iam" {
  source = "../../modules/iam"

  name                 = local.name
  oidc_provider_arn    = module.eks.oidc_provider_arn
  oidc_provider_url    = module.eks.oidc_provider_url
  namespace            = var.k8s_namespace
  service_account_name = var.k8s_service_account_name
  secret_arn           = module.secrets.secret_arn
  enable_github_oidc   = var.enable_github_oidc
  github_org           = var.github_org
  github_repo          = var.github_repo
  ecr_repository_arn   = module.ecr.repository_arn
  tags                 = local.common_tags
}
