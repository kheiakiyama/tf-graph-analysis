terraform {
  required_version = ">= 1.5.0"
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 3.0"
    }
  }

  backend "azurerm" {
    # Backend configuration (resource_group_name, storage_account_name, container_name, key)
    # is provided at runtime via -backend-config or GitHub Actions secrets to prevent
    # hardcoding Azure resource names in public repositories.
  }
}

provider "azurerm" {
  features {}
}
