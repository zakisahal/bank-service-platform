resource "aws_ecr_repository" "this" {
  name                 = var.name
  image_tag_mutability = "IMMUTABLE" # regulated env: a tag must always point at the same digest

  image_scanning_configuration {
    scan_on_push = true
  }

  encryption_configuration {
    encryption_type = "KMS"
    kms_key         = var.kms_key_arn
  }

  tags = var.tags
}

# Expire untagged images (failed/superseded builds) after 14 days; keep all
# tagged images since git-sha tags are the audit trail of what ran in prod.
resource "aws_ecr_lifecycle_policy" "this" {
  repository = aws_ecr_repository.this.name

  policy = jsonencode({
    rules = [{
      rulePriority = 1
      description  = "Expire untagged images after 14 days"
      selection = {
        tagStatus   = "untagged"
        countType   = "sinceImagePushed"
        countUnit   = "days"
        countNumber = 14
      }
      action = { type = "expire" }
    }]
  })
}

# No repository policy: EKS node pull access comes from the
# AmazonEC2ContainerRegistryReadOnly managed policy on the node IAM role
# (see modules/iam), which is the standard EKS pattern rather than a
# resource-based policy here.
