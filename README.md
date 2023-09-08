# Terraform Provider Tecton

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.19
- [Tecton CLI](https://docs.tecton.ai/docs/setting-up-tecton/development-setup/installing-the-tecton-cli) == 0.7.3

## Building The Provider

1. Clone the repository
1. Enter the repository directory
1. Build the provider using the Go `install` command:

```shell
go install
```

## Adding Dependencies

This provider uses [Go modules](https://github.com/golang/go/wiki/Modules).
Please see the Go documentation for the most up to date information about using Go modules.

To add a new dependency `github.com/author/dependency` to your Terraform provider:

```shell
go get github.com/author/dependency
go mod tidy
```

Then commit the changes to `go.mod` and `go.sum`.

## Using the provider

Fill this in for each provider

## Developing the Provider

If you wish to work on the provider, you'll first need [Go](http://www.golang.org) installed on your machine (see [Requirements](#requirements) above).

To compile the provider, run `go install`. This will build the provider and put the provider binary in the `$GOPATH/bin` directory.

To generate or update documentation, run `go generate`.

In order to run the full suite of Acceptance tests, run `make testacc` with the environment variables specified below.

*Note:* Acceptance tests create real resources, and often cost money to run.

```shell
TF_VAR_tecton_api_key=<your-tecton-api-key> \
TF_VAR_tecton_url=<your-tecton-url> \
TF_VAR_tecton_service_account_existing_roles=<your-tecton-service-account-id-with-existing-roles> \
TF_VAR_tecton_service_account_no_existing_roles=<your-tecton-service-account-id-with-no-existing-roles> \
make testacc
```
