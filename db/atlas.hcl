env "local" {
  src = [
    "file://schema.pg.hcl",
  ]
  url = "postgres://myuser:mypassword@localhost:5433/mydatabase?sslmode=disable"
  dev = "docker://postgres/16/dev"
}

env  {
  migration  {
    baseline = "20240525191742"
  }
}
