terraform {
  # Backend is configured at init time via -backend-config flags:
  #   -backend-config="storage_account_name=<storage-account>"
  #   -backend-config="container_name=tfstate"
  #   -backend-config="key=azuresql/<server-name>/terraform.tfstate"
  #   -backend-config="resource_group_name=<resource-group>"
  backend "azurerm" {}

  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 4.0"
    }
  }

  required_version = ">= 1.9"
}

provider "azurerm" {
  features {}
}

# ── Resource Group ──────────────────────────────────────────────────────────────
# Read an existing resource group — the workflow assumes the user already has one
# and may not have permission to create new resource groups.

data "azurerm_resource_group" "this" {
  name = var.resource_group_name
}

# ── SQL Server ──────────────────────────────────────────────────────────────────

resource "azurerm_mssql_server" "this" {
  name                         = var.server_name
  resource_group_name          = data.azurerm_resource_group.this.name
  location                     = data.azurerm_resource_group.this.location
  version                      = "12.0"
  administrator_login          = var.admin_username
  administrator_login_password = var.admin_password

  tags = {
    ManagedBy = "openchoreo-workflow"
  }
}

# ── Firewall rule — allow Azure services and all IPs (public sample) ────────────
# The 0.0.0.0 – 255.255.255.255 range is open so you can connect directly from
# your workstation to verify the database after provisioning.
# For production use, restrict to your application's IP range.

resource "azurerm_mssql_firewall_rule" "allow_azure_services" {
  name             = "AllowAzureServices"
  server_id        = azurerm_mssql_server.this.id
  start_ip_address = "0.0.0.0"
  end_ip_address   = "0.0.0.0"
}

resource "azurerm_mssql_firewall_rule" "allow_all" {
  name             = "AllowAll"
  server_id        = azurerm_mssql_server.this.id
  start_ip_address = "0.0.0.0"
  end_ip_address   = "255.255.255.255"
}

# ── SQL Database ────────────────────────────────────────────────────────────────

resource "azurerm_mssql_database" "this" {
  name      = var.db_name
  server_id = azurerm_mssql_server.this.id
  sku_name  = var.db_sku

  tags = {
    ManagedBy = "openchoreo-workflow"
  }
}
