variable "resource_group_name" {
  type        = string
  description = "The name of the test resource group."
  default     = "rg-tf-graph-analysis-tfc-test"
}

variable "location" {
  type        = string
  description = "The Azure region for test deployment."
  default     = "japaneast"
}

variable "environment" {
  type        = string
  description = "The environment tag."
  default     = "tfc-test"
}
