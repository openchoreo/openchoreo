terraform {
  # Backend is configured at init time via -backend-config flags:
  #   -backend-config="bucket=<s3-bucket>"
  #   -backend-config="key=rds/<db-identifier>/terraform.tfstate"
  #   -backend-config="region=<aws-region>"
  backend "s3" {}

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }

  required_version = ">= 1.9"
}

provider "aws" {
  region = var.aws_region
}

# ── Networking — use the default VPC's public subnets ────────────────────────
# The default VPC has public subnets with a route to the internet gateway,
# which is required for the publicly_accessible flag below to take effect.

data "aws_vpc" "default" {
  default = true
}

data "aws_subnets" "default" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.default.id]
  }
}

resource "aws_db_subnet_group" "this" {
  name        = "${var.db_identifier}-subnet-group"
  subnet_ids  = data.aws_subnets.default.ids
  description = "Subnet group for ${var.db_identifier}"
}

# ── Security group — allow PostgreSQL from anywhere (public sample) ───────────
# Port 5432 is open to 0.0.0.0/0 so you can connect directly from your
# workstation to verify the instance after provisioning.
# For production use, restrict cidr_blocks to your application's IP range.

resource "aws_security_group" "rds" {
  name        = "${var.db_identifier}-rds-sg"
  description = "Allow PostgreSQL access from anywhere (sample/test use)"
  vpc_id      = data.aws_vpc.default.id

  ingress {
    description = "PostgreSQL"
    from_port   = 5432
    to_port     = 5432
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

# ── RDS PostgreSQL instance ───────────────────────────────────────────────────

resource "aws_db_instance" "this" {
  identifier        = var.db_identifier
  engine            = "postgres"
  engine_version    = var.db_engine_version
  instance_class    = "db.t3.micro"  # free-tier eligible
  allocated_storage = 20             # minimum allowed by AWS for PostgreSQL RDS
  storage_type      = "gp2"

  db_name  = var.db_name
  username = var.db_username
  password = var.db_password

  db_subnet_group_name   = aws_db_subnet_group.this.name
  vpc_security_group_ids = [aws_security_group.rds.id]

  publicly_accessible = true
  multi_az            = false
  storage_encrypted   = false
  deletion_protection = false
  skip_final_snapshot = true

  tags = {
    ManagedBy = "openchoreo-workflow"
  }
}
