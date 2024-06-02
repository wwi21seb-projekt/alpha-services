env "local" {
  src = [
    "file://schema.pg.hcl",
  ]
  url = "postgres://alpha_user:A3C2EW7ieV@localhost:5432/alpha_db?sslmode=disable"
  dev = "docker://postgres/16/dev"
}

env  {
  migration  {
    baseline = "20240525191742"
  }
}
