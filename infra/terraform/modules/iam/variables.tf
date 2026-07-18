variable "name" {
  type = string
}

variable "oidc_provider_arn" {
  type        = string
  description = "ARN of the EKS cluster's OIDC identity provider (from modules/eks)."
}

variable "oidc_provider_url" {
  type        = string
  description = "EKS OIDC issuer URL without the https:// prefix (from modules/eks)."
}

variable "namespace" {
  type    = string
  default = "default"
}

variable "service_account_name" {
  type    = string
  default = "bank-service"
}

variable "secret_arn" {
  type        = string
  description = "Secrets Manager secret the app's pod is allowed to read."
}

variable "enable_github_oidc" {
  type        = bool
  default     = false
  description = "Create the GitHub Actions OIDC provider + CI deploy role. Left off by default since it registers an account-wide OIDC provider - enable once per AWS account, not per environment."
}

variable "github_org" {
  type    = string
  default = ""
}

variable "github_repo" {
  type    = string
  default = ""
}

variable "ecr_repository_arn" {
  type    = string
  default = ""
}

variable "tags" {
  type    = map(string)
  default = {}
}
