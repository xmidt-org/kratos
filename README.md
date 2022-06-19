# kratos, the client emulator

[![Build Status](https://github.com/xmidt-org/kratos/actions/workflows/ci.yml/badge.svg)](https://github.com/xmidt-org/kratos/actions/workflows/ci.yml)
[![Dependency Updateer](https://github.com/xmidt-org/kratos/actions/workflows/updater.yml/badge.svg)](https://github.com/xmidt-org/kratos/actions/workflows/updater.yml)
[![codecov.io](http://codecov.io/github/xmidt-org/kratos/coverage.svg?branch=main)](http://codecov.io/github/xmidt-org/kratos?branch=main)
[![Go Report Card](https://goreportcard.com/badge/github.com/xmidt-org/kratos)](https://goreportcard.com/report/github.com/xmidt-org/kratos)
[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=xmidt-org_kratos&metric=alert_status)](https://sonarcloud.io/dashboard?id=xmidt-org_kratos)
[![Apache V2 License](http://img.shields.io/badge/license-Apache%20V2-blue.svg)](https://github.com/xmidt-org/kratos/blob/main/LICENSE)
[![GitHub Release](https://img.shields.io/github/release/xmidt-org/kratos.svg)](CHANGELOG.md)
[![GoDoc](https://pkg.go.dev/badge/github.com/xmidt-org/kratos)](https://pkg.go.dev/github.com/xmidt-org/kratos)

Websocket wrapper that provides a simple API for making new connections,
sending messages on that connection, and providing a way to handle received
messages.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [How to Install](#how-to-install)
- [Contributing](#contributing)

## Code of Conduct

This project and everyone participating in it are governed by the [XMiDT Code Of Conduct](https://xmidt.io/code_of_conduct/). 
By participating, you agree to this Code.

## How to Install
This project uses go modules to manage its dependencies. This is best used with go 1.12+.  to import this module, run:
```
go get github.com/xmidt-org/kratos@latest
```
or add it to your go.mod file for your project.

## Contributing

Refer to [CONTRIBUTING.md](CONTRIBUTING.md).
