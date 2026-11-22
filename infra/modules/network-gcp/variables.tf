variable "project_id" { type = string }
variable "region" {
  type    = string
  default = "us-west1"
}
variable "name" {
  type    = string
  default = "samplier"
}
variable "tags" { type = map(string) }
