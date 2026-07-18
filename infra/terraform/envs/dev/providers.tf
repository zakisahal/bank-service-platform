provider "aws" {
  region = var.aws_region

  # This repo is authored/validated/planned only (see brief) - there are no
  # real AWS credentials wired up. These three flags stop the provider from
  # calling STS at plan time so `terraform plan` succeeds offline. Once
  # this stack is applied for real, CI authenticates via the
  # terraform_plan / terraform_apply OIDC roles in modules/iam (see
  # ci.yaml's "terraform" job and DECISIONS.md) and these flags are
  # removed - they exist only for this exercise's offline validation, not
  # as a stand-in for real CI credentials.
  skip_credentials_validation = var.offline_demo
  skip_requesting_account_id  = var.offline_demo
  skip_metadata_api_check     = var.offline_demo

  default_tags {
    tags = local.common_tags
  }
}
