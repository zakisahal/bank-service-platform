# --- IRSA: bank-service pod -> Secrets Manager -------------------------

data "aws_iam_policy_document" "irsa_assume" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [var.oidc_provider_arn]
    }

    condition {
      test     = "StringEquals"
      variable = "${var.oidc_provider_url}:sub"
      values   = ["system:serviceaccount:${var.namespace}:${var.service_account_name}"]
    }

    condition {
      test     = "StringEquals"
      variable = "${var.oidc_provider_url}:aud"
      values   = ["sts.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "irsa" {
  name               = "${var.name}-irsa"
  assume_role_policy = data.aws_iam_policy_document.irsa_assume.json
  tags               = var.tags
}

data "aws_iam_policy_document" "irsa_permissions" {
  statement {
    effect    = "Allow"
    actions   = ["secretsmanager:GetSecretValue", "secretsmanager:DescribeSecret"]
    resources = [var.secret_arn]
  }
}

resource "aws_iam_role_policy" "irsa" {
  name   = "${var.name}-irsa-secrets"
  role   = aws_iam_role.irsa.id
  policy = data.aws_iam_policy_document.irsa_permissions.json
}

# --- CI/CD deploy role: GitHub Actions OIDC -> ECR push ----------------
#
# No long-lived AWS access keys in CI. GitHub's OIDC token is federated
# directly to a role scoped to this repo, this branch, pushing images to
# this one ECR repository only. The role deliberately cannot touch EKS or
# Secrets Manager - deploys happen via GitOps (Argo CD reconciling the
# cluster), not by CI holding cluster-admin-adjacent credentials. See
# DECISIONS.md.

resource "aws_iam_openid_connect_provider" "github" {
  count = var.enable_github_oidc ? 1 : 0

  url             = "https://token.actions.githubusercontent.com"
  client_id_list  = ["sts.amazonaws.com"]
  thumbprint_list = ["6938fd4d98bab03faadb97b34396831e3780aea1"]

  tags = var.tags
}

data "aws_iam_policy_document" "ci_assume" {
  count = var.enable_github_oidc ? 1 : 0

  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [aws_iam_openid_connect_provider.github[0].arn]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values   = ["sts.amazonaws.com"]
    }

    condition {
      test     = "StringLike"
      variable = "token.actions.githubusercontent.com:sub"
      values   = ["repo:${var.github_org}/${var.github_repo}:ref:refs/heads/main"]
    }
  }
}

resource "aws_iam_role" "ci_deploy" {
  count = var.enable_github_oidc ? 1 : 0

  name               = "${var.name}-ci-deploy"
  assume_role_policy = data.aws_iam_policy_document.ci_assume[0].json
  tags               = var.tags
}

data "aws_iam_policy_document" "ci_permissions" {
  count = var.enable_github_oidc ? 1 : 0

  statement {
    effect    = "Allow"
    actions   = ["ecr:GetAuthorizationToken"]
    resources = ["*"] # this action is only ever authorized at the account level, not per-repo
  }

  statement {
    effect = "Allow"
    actions = [
      "ecr:BatchCheckLayerAvailability",
      "ecr:PutImage",
      "ecr:InitiateLayerUpload",
      "ecr:UploadLayerPart",
      "ecr:CompleteLayerUpload",
      "ecr:BatchGetImage",
    ]
    resources = [var.ecr_repository_arn]
  }
}

resource "aws_iam_role_policy" "ci_deploy" {
  count = var.enable_github_oidc ? 1 : 0

  name   = "${var.name}-ci-ecr-push"
  role   = aws_iam_role.ci_deploy[0].id
  policy = data.aws_iam_policy_document.ci_permissions[0].json
}

# --- Terraform plan role: GitHub Actions OIDC -> read-only -------------
#
# Separate from ci_deploy above on purpose: that role can push container
# images, this one can only read AWS state to compute a plan. Neither role
# can do what the other does. Trusted to *any* pull_request run against
# this repo (not just main) since it's read-only and therefore safe to run
# unattended on every PR - see ci.yaml's "terraform" job, which runs a real
# `terraform plan` through this role when TF_PLAN_ROLE_ARN is configured,
# and skips that step (falling back to the offline `validate`-only path)
# otherwise, so the workflow stays usable before that repo variable exists.

data "aws_iam_policy_document" "terraform_plan_assume" {
  count = var.enable_github_oidc ? 1 : 0

  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [aws_iam_openid_connect_provider.github[0].arn]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values   = ["sts.amazonaws.com"]
    }

    condition {
      test     = "StringLike"
      variable = "token.actions.githubusercontent.com:sub"
      values   = ["repo:${var.github_org}/${var.github_repo}:pull_request"]
    }
  }
}

resource "aws_iam_role" "terraform_plan" {
  count = var.enable_github_oidc ? 1 : 0

  name               = "${var.name}-terraform-plan"
  assume_role_policy = data.aws_iam_policy_document.terraform_plan_assume[0].json
  tags               = var.tags
}

# Describe/List/Get only, scoped to the services this stack's modules
# actually touch - deliberately narrower than the AWS-managed
# ReadOnlyAccess policy, and deliberately excludes
# secretsmanager:GetSecretValue: planning needs to know a secret exists,
# never what's in it.
data "aws_iam_policy_document" "terraform_plan_permissions" {
  count = var.enable_github_oidc ? 1 : 0

  statement {
    effect = "Allow"
    actions = [
      "ec2:Describe*",
      "eks:Describe*",
      "eks:List*",
      "rds:Describe*",
      "rds:List*",
      "rds:ListTagsForResource",
      "iam:Get*",
      "iam:List*",
      "kms:Describe*",
      "kms:List*",
      "kms:GetKeyPolicy",
      "kms:GetKeyRotationStatus",
      "secretsmanager:Describe*",
      "secretsmanager:List*",
      "ecr:Describe*",
      "ecr:List*",
      "ecr:GetRepositoryPolicy",
      "ecr:GetLifecyclePolicy",
      "logs:Describe*",
      "logs:List*",
      "logs:GetLogGroupFields",
      "sts:GetCallerIdentity",
    ]
    resources = ["*"] # read-only Describe/List/Get actions - none of these APIs support resource-level scoping
  }
}

resource "aws_iam_role_policy" "terraform_plan" {
  count = var.enable_github_oidc ? 1 : 0

  name   = "${var.name}-terraform-plan-readonly"
  role   = aws_iam_role.terraform_plan[0].id
  policy = data.aws_iam_policy_document.terraform_plan_permissions[0].json
}

# --- Terraform apply role: GitHub Actions OIDC -> write, gated ---------
#
# Broader than terraform_plan because apply has to be able to create every
# resource type these modules define - but never applied in this exercise
# (see DECISIONS.md), and trusted only to a dedicated "terraform-apply"
# GitHub Environment (Settings > Environments), which is where a required
# reviewer is configured - the same manual-approval pattern used for the
# production deploy gate in cd.yaml, applied here to infrastructure changes
# instead of application deploys.
#
# IAM permissions are the one place this role is scoped by resource ARN
# rather than by service wildcard: a role that can freely CreateRole /
# AttachRolePolicy on *any* role is a privilege-escalation path (it could
# grant itself broader access than it started with), so iam:* here is
# restricted to role/policy names matching this stack's own naming
# convention (${var.name}-*), including a scoped iam:PassRole. The
# residual risk this doesn't close: this role can still create a
# **new**, differently-named IAM role and attach arbitrary policies to
# *that* - closing that fully needs a permissions boundary applied to
# every role this pipeline creates, which is not implemented here and is
# called out in DECISIONS.md as a known gap, not an oversight.

data "aws_iam_policy_document" "terraform_apply_assume" {
  count = var.enable_github_oidc ? 1 : 0

  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [aws_iam_openid_connect_provider.github[0].arn]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values   = ["sts.amazonaws.com"]
    }

    condition {
      test     = "StringLike"
      variable = "token.actions.githubusercontent.com:sub"
      values   = ["repo:${var.github_org}/${var.github_repo}:environment:terraform-apply"]
    }
  }
}

resource "aws_iam_role" "terraform_apply" {
  count = var.enable_github_oidc ? 1 : 0

  name               = "${var.name}-terraform-apply"
  assume_role_policy = data.aws_iam_policy_document.terraform_apply_assume[0].json
  tags               = var.tags
}

data "aws_iam_policy_document" "terraform_apply_permissions" {
  count = var.enable_github_oidc ? 1 : 0

  # Non-IAM services this stack manages: full CRUD, resource "*" - these
  # resources don't exist yet at the time this policy is evaluated, so
  # there's nothing to scope an ARN to upfront. This is the standard
  # shape of a Terraform apply role: broad within the services it owns,
  # tight on IAM specifically (see below).
  statement {
    effect = "Allow"
    actions = [
      "ec2:*",
      "eks:*",
      "rds:*",
      "kms:*",
      "secretsmanager:*",
      "ecr:*",
      "logs:*",
      "sts:GetCallerIdentity",
    ]
    resources = ["*"]
  }

  statement {
    effect = "Allow"
    actions = [
      "iam:CreateRole",
      "iam:DeleteRole",
      "iam:GetRole",
      "iam:TagRole",
      "iam:UntagRole",
      "iam:PutRolePolicy",
      "iam:DeleteRolePolicy",
      "iam:GetRolePolicy",
      "iam:ListRolePolicies",
      "iam:AttachRolePolicy",
      "iam:DetachRolePolicy",
      "iam:ListAttachedRolePolicies",
      "iam:CreateOpenIDConnectProvider",
      "iam:DeleteOpenIDConnectProvider",
      "iam:GetOpenIDConnectProvider",
      "iam:TagOpenIDConnectProvider",
    ]
    resources = [
      "arn:aws:iam::*:role/${var.name}-*",
      "arn:aws:iam::*:oidc-provider/token.actions.githubusercontent.com",
    ]
  }

  statement {
    effect  = "Allow"
    actions = ["iam:PassRole"]
    resources = [
      "arn:aws:iam::*:role/${var.name}-*",
    ]
    condition {
      test     = "StringEquals"
      variable = "iam:PassedToService"
      values   = ["eks.amazonaws.com", "ec2.amazonaws.com", "rds.amazonaws.com"]
    }
  }
}

resource "aws_iam_role_policy" "terraform_apply" {
  count = var.enable_github_oidc ? 1 : 0

  name   = "${var.name}-terraform-apply-write"
  role   = aws_iam_role.terraform_apply[0].id
  policy = data.aws_iam_policy_document.terraform_apply_permissions[0].json
}
