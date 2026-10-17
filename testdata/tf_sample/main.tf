terraform {
  required_version = ">= 1.5.0"
}

resource "terraform_data" "root_a" {
  input = "root_a"
}

resource "terraform_data" "root_b" {
  input = "root_b"
}

resource "terraform_data" "middle_fast" {
  input = terraform_data.root_a.output
}

resource "terraform_data" "slow_step_1" {
  input = terraform_data.root_b.output
}

resource "terraform_data" "slow_step_2" {
  input = terraform_data.slow_step_1.output
}

resource "terraform_data" "leaf" {
  input = [
    terraform_data.middle_fast.output,
    terraform_data.slow_step_2.output
  ]
}
