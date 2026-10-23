variable "resource_group_name" {
  type        = string
  description = "The name of the resource group."
}

variable "location" {
  type        = string
  description = "The Azure region where resources will be created."
  default     = "japaneast"
}

variable "environment" {
  type        = string
  description = "Environment identifier (e.g. test, dev, prod)."
  default     = "test"
}
