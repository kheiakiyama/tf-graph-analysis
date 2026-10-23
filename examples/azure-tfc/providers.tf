terraform {
  required_version = ">= 1.5.0"
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 3.0"
    }
  }

  cloud {
    # Replace with your HCP Terraform organization name
    organization = "your-tfc-org"

    workspaces {
      # Replace with your HCP Terraform workspace name
      name = "your-tfc-workspace"
    }
  }
}

provider "azurerm" {
  features {}
}
